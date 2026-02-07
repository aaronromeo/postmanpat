package watchrunner

import (
	"context"
	"log/slog"
	"strings"

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/aaronromeo/postmanpat/internal/imapclient"
	"github.com/aaronromeo/postmanpat/internal/matchers"
)

type Deps struct {
	Ctx      context.Context
	Client   *imapclient.Client
	Rules    []config.Rule
	Log      *slog.Logger
	Announce func(string)
}

type State struct {
	LastUID   uint32
	LastCount uint32
}

func ProcessUIDs(deps Deps, state *State, uids []uint32) error {
	deps.Log.Debug("search newer than uid", "last_uid", state.LastUID, "uids", len(uids))
	if len(uids) == 0 {
		return nil
	}
	data, err := deps.Client.FetchSenderData(deps.Ctx, uids)
	if err != nil {
		return err
	}
	deps.Log.Debug("fetched messages for processing", "messages", len(data))
	for _, message := range data {
		matchedAny := false
		for _, rule := range deps.Rules {
			ok, err := matchers.MatchesClient(rule.Client, matchers.ClientMessage{
				ListID:         message.ListID,
				SenderDomains:  message.SenderDomains,
				ReplyToDomains: message.ReplyToDomains,
				SubjectRaw:     message.SubjectRaw,
				Recipients:     message.Recipients,
			})
			if err != nil {
				return err
			}
			if ok {
				matchedAny = true
				deps.Log.Info("rule matched", "rule", rule.Name, "list_id", message.ListID)
				if deps.Announce != nil {
					deps.Announce(rule.Name)
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

func Reconnect(deps Deps, state *State, mailbox string) error {
	_ = deps.Client.Close()
	if err := deps.Client.Connect(); err != nil {
		return err
	}
	selection, err := deps.Client.SelectMailbox(deps.Ctx, mailbox)
	if err != nil {
		return err
	}
	deps.Log.Info("reconnected", "mailbox", mailbox, "messages", selection.NumMessages)
	uids, err := deps.Client.SearchUIDsNewerThan(deps.Ctx, state.LastUID)
	if err != nil {
		return err
	}
	if err := ProcessUIDs(deps, state, uids); err != nil {
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
