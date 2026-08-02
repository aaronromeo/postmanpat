package watchrunner

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/watchrunner/internal/matchers"
	giimap "github.com/emersion/go-imap/v2"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
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

type watchInstruments struct {
	messages       metric.Int64Counter
	ruleMatches    metric.Int64Counter
	actionMessages metric.Int64Counter
}

var watchInstrumentsOnce = sync.OnceValue(func() watchInstruments {
	meter := otel.Meter("github.com/aaronromeo/postmanpat/watchrunner")
	messages, _ := meter.Int64Counter("postmanpat.watch.messages.processed", metric.WithUnit("{message}"))
	ruleMatches, _ := meter.Int64Counter("postmanpat.watch.rule.matches", metric.WithUnit("{message}"))
	actionMessages, _ := meter.Int64Counter("postmanpat.watch.action.messages", metric.WithUnit("{message}"))
	return watchInstruments{messages: messages, ruleMatches: ruleMatches, actionMessages: actionMessages}
})

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
	tracer := otel.Tracer("github.com/aaronromeo/postmanpat/watchrunner")
	for _, message := range data {
		messageCtx, msgSpan := tracer.Start(deps.Ctx, "watch.message",
			trace.WithAttributes(
				attribute.Int64("imap.uid", int64(message.UID)),
				attribute.String("email.message_id", message.MessageID),
				attribute.StringSlice("email.from", message.From),
				attribute.String("email.subject", message.SubjectRaw),
				attribute.String("email.internal_date", message.MessageDate.UTC().Format(time.RFC3339)),
			))
		watchInstrumentsOnce().messages.Add(messageCtx, 1)

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
				msgSpan.RecordError(err)
				msgSpan.SetStatus(codes.Error, err.Error())
				msgSpan.End()
				return err
			}
			msgSpan.AddEvent("watch.rule_evaluated",
				trace.WithAttributes(
					attribute.String("rule.name", rule.Name),
					attribute.Bool("matched", ok),
				))
			if ok {
				matchedAny = true
				watchInstrumentsOnce().ruleMatches.Add(messageCtx, 1,
					metric.WithAttributes(attribute.String("rule.name", rule.Name)))
				deps.Log.InfoContext(messageCtx, "rule matched", "rule", rule.Name, "list_id", message.ListID)
				if deps.Announce != nil {
					deps.Announce(rule.Name)
				}
				if err := applyActions(messageCtx, client, deps, rule, message.UID); err != nil {
					msgSpan.RecordError(err)
					msgSpan.SetStatus(codes.Error, err.Error())
					msgSpan.End()
					return err
				}
			}
		}
		if !matchedAny {
			deps.Log.InfoContext(messageCtx, "no rule matched")
			if deps.Announce != nil {
				deps.Announce("")
			}
		}
		msgSpan.End()
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
	tracer := otel.Tracer("github.com/aaronromeo/postmanpat/watchrunner")
	for _, action := range rule.Actions {
		actionCtx, span := tracer.Start(ctx, "watch.action",
			trace.WithAttributes(
				attribute.String("rule.name", rule.Name),
				attribute.String("action.type", string(action.Type)),
				attribute.String("action.destination", action.Destination),
			))
		var err error
		switch action.Type {
		case appconfig.DELETE:
			expungeAfterDelete := true
			if action.ExpungeAfterDelete != nil {
				expungeAfterDelete = *action.ExpungeAfterDelete
			}
			err = client.DeleteUIDs(actionCtx, []uint32{uid}, expungeAfterDelete)
		case appconfig.MOVE:
			destination := strings.TrimSpace(action.Destination)
			if destination == "" {
				err = fmt.Errorf("Action move missing destination for rule %q", rule.Name)
			} else {
				err = client.MoveUIDs(actionCtx, []uint32{uid}, destination)
			}
		default:
			err = fmt.Errorf("unsupported action type %q for rule %q", action.Type, rule.Name)
		}
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			return err
		}
		watchInstrumentsOnce().actionMessages.Add(actionCtx, 1, metric.WithAttributes(
			attribute.String("action.type", string(action.Type)),
			attribute.String("rule.name", rule.Name),
			attribute.String("destination", action.Destination),
		))
		span.End()
	}
	return nil
}
