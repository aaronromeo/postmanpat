package watchrunner

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aaronromeo/postmanpat/envmgr"
	"github.com/aaronromeo/postmanpat/watchrunner/internal/matchers"
)

type TestMatch struct {
	MessageDate    time.Time
	SubjectRaw     string
	ListID         string
	ReplyToDomains []string
	SenderDomains  []string
	Recipients     []string
}

func RunRuleTest(ctx context.Context, client WatchRunner, rule envmgr.Rule, mailbox string, limit int) ([]TestMatch, error) {
	if rule.Client == nil {
		return nil, fmt.Errorf("rule %q does not define client matchers", rule.Name)
	}
	if strings.TrimSpace(mailbox) == "" {
		return nil, errors.New("mailbox is required")
	}
	if limit <= 0 {
		limit = 10
	}

	if _, err := client.SelectMailbox(ctx, mailbox); err != nil {
		return nil, err
	}

	uids, err := client.SearchUIDsNewerThan(ctx, 0)
	if err != nil {
		return nil, err
	}
	if len(uids) == 0 {
		return nil, nil
	}

	out := make([]TestMatch, 0, limit)
	chunkSize := 200
	for end := len(uids); end > 0 && len(out) < limit; end -= chunkSize {
		start := end - chunkSize
		if start < 0 {
			start = 0
		}
		batch := uids[start:end]
		data, err := client.FetchSenderData(ctx, batch)
		if err != nil {
			return nil, err
		}
		sort.Slice(data, func(i, j int) bool {
			return data[i].MessageDate.After(data[j].MessageDate)
		})
		for _, message := range data {
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
				return nil, err
			}
			if !ok {
				continue
			}
			out = append(out, TestMatch{
				MessageDate:    message.MessageDate,
				SubjectRaw:     message.SubjectRaw,
				ListID:         message.ListID,
				ReplyToDomains: message.ReplyToDomains,
				SenderDomains:  message.SenderDomains,
				Recipients:     message.Recipients,
			})
			if len(out) >= limit {
				break
			}
		}
	}

	return out, nil
}
