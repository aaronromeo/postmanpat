package watchrunner

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/watchrunner/internal/matchers"
	giimap "github.com/emersion/go-imap/v2"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
)

type WatchRunner interface {
	Connect() error
	Close() error
	Idle() (*giimapclient.IdleCommand, error)
	SelectMailbox(ctx context.Context, mailbox string) (*giimap.SelectData, error)
	FetchSenderData(ctx context.Context, uids []uint32) ([]imap.MailData, error)
	SearchUIDsNewerThan(ctx context.Context, lastUID uint32) ([]uint32, error)
	MoveUIDs(ctx context.Context, uids []uint32, destination string) error
	DeleteUIDs(ctx context.Context, uids []uint32, expunge bool) error
}

type Client struct {
	*imap.Client
}

type Deps struct {
	Ctx      context.Context
	Rules    []appconfig.Rule
	Log      *slog.Logger
	Announce func(string)
}

type State struct {
	LastUID   uint32
	LastCount uint32
}

func New(opts ...imap.Option) *Client {
	return &Client{Client: imap.NewWatch(opts...)}
}

func ProcessUIDs(client WatchRunner, deps Deps, state *State, uids []uint32) error {
	deps.Log.Debug("search newer than uid", "last_uid", state.LastUID, "uids", len(uids))
	if len(uids) == 0 {
		return nil
	}
	data, err := client.FetchSenderData(deps.Ctx, uids)
	if err != nil {
		return err
	}
	deps.Log.Debug("fetched messages for processing", "messages", len(data))
	for _, message := range data {
		matchedAny := false
		for _, rule := range deps.Rules {
			ok, err := (matchers.ClientMessage{
				ListID:           message.ListID,
				SenderDomains:    message.SenderDomains,
				ReplyToDomains:   message.ReplyToDomains,
				SubjectRaw:       message.SubjectRaw,
				Recipients:       message.Recipients,
				RecipientTags:    message.RecipientTags,
				Body:             message.Body,
				Cc:               message.Cc,
				ReturnPathDomain: message.ReturnPathDomain,
				ListUnsubscribe:  message.ListUnsubscribe,
			}).Match(rule.Client)
			if err != nil {
				return err
			}
			if ok {
				matchedAny = true
				deps.Log.Info("rule matched", "rule", rule.Name, "list_id", message.ListID)
				if deps.Announce != nil {
					deps.Announce(rule.Name)
				}
				if err := applyActions(deps.Ctx, client, deps, rule, message.UID); err != nil {
					return err
				}
			}
		}
		if !matchedAny {
			deps.Log.Info("no rule matched")
			if deps.Announce != nil {
				deps.Announce("")
			}
		}
	}
	state.LastUID = maxUID(state.LastUID, uids)
	deps.Log.Debug("updated last uid", "last_uid", state.LastUID)
	return nil
}

func Reconnect(client WatchRunner, deps Deps, state *State, mailbox string) error {
	_ = client.Close()
	if err := client.Connect(); err != nil {
		return err
	}
	selection, err := client.SelectMailbox(deps.Ctx, mailbox)
	if err != nil {
		return err
	}
	deps.Log.Info("reconnected", "mailbox", mailbox, "messages", selection.NumMessages)
	uids, err := client.SearchUIDsNewerThan(deps.Ctx, state.LastUID)
	if err != nil {
		return err
	}
	if err := ProcessUIDs(client, deps, state, uids); err != nil {
		return err
	}
	state.LastCount = selection.NumMessages
	return nil
}

func IsBenignIdleError(err error) bool {
	if err == nil {
		return true
	}
	return strings.Contains(err.Error(), "use of closed network connection")
}

func maxUID(current uint32, uids []uint32) uint32 {
	max := current
	for _, uid := range uids {
		if uid > max {
			max = uid
		}
	}
	return max
}

func applyActions(ctx context.Context, client WatchRunner, deps Deps, rule appconfig.Rule, uid uint32) error {
	if uid == 0 {
		return nil
	}
	for _, action := range rule.Actions {
		switch action.Type {
		case appconfig.DELETE:
			expungeAfterDelete := true
			if action.ExpungeAfterDelete != nil {
				expungeAfterDelete = *action.ExpungeAfterDelete
			}
			if err := client.DeleteUIDs(ctx, []uint32{uid}, expungeAfterDelete); err != nil {
				return err
			}
		case appconfig.MOVE:
			destination := strings.TrimSpace(action.Destination)
			if destination == "" {
				return fmt.Errorf("Action move missing destination for rule %q", rule.Name)
			}
			if err := client.MoveUIDs(ctx, []uint32{uid}, destination); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported action type %q for rule %q", action.Type, rule.Name)
		}
	}
	return nil
}
