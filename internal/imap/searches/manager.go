package searches

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/emersion/go-imap/v2"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
)

type ServerSearcher interface {
	SearchByServerMatchers(ctx context.Context, matchers config.ServerMatchers) (map[string][]uint32, error)
}

type ClientSearcher interface {
	SearchUIDsNewerThan(ctx context.Context, lastUID uint32) ([]uint32, error)
}

// Interface to initialize the manager
type ClientProvider interface {
	IMAPClient() *giimapclient.Client
}

type IMAPSearchManager struct {
	provider func() *giimapclient.Client
}

func New(provider ClientProvider) *IMAPSearchManager {
	return &IMAPSearchManager{provider: provider.IMAPClient}
}

// SearchByServerMatchers returns UIDs for messages matching the provided matchers via IMAP SEARCH.
// Results are grouped by mailbox to avoid UID collisions across folders.
func (m *IMAPSearchManager) SearchByServerMatchers(ctx context.Context, matchers config.ServerMatchers) (map[string][]uint32, error) {
	if m.provider == nil {
		return nil, errors.New("IMAP client is not connected")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(matchers.Folders) == 0 {
		return nil, errors.New("matcher Folders is required")
	}

	matches := make(map[string][]uint32)
	for _, folder := range matchers.Folders {
		folder = strings.TrimSpace(folder)
		if folder == "" {
			return nil, errors.New("matcher Folder is required")
		}

		if _, err := m.provider().Select(folder, nil).Wait(); err != nil {
			return nil, err
		}

		criteria, err := buildSearchCriteria(matchers)
		if err != nil {
			return nil, err
		}

		data, err := m.provider().UIDSearch(criteria, nil).Wait()
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		uids := data.AllUIDs()
		if len(uids) == 0 {
			continue
		}
		mailboxUIDs := make([]uint32, 0, len(uids))
		for _, uid := range uids {
			mailboxUIDs = append(mailboxUIDs, uint32(uid))
		}
		matches[folder] = mailboxUIDs
	}

	return matches, nil
}

func buildSearchCriteria(matchers config.ServerMatchers) (*imap.SearchCriteria, error) {
	criteria := &imap.SearchCriteria{}
	criteria.NotFlag = append(criteria.NotFlag, imap.FlagDeleted)

	if matchers.AgeWindow != nil && !matchers.AgeWindow.IsEmpty() {
		if strings.TrimSpace(matchers.AgeWindow.Max) != "" {
			dur, err := config.ParseRelativeDuration(matchers.AgeWindow.Max)
			if err != nil {
				return nil, fmt.Errorf("invalid age_window.max: %w", err)
			}
			criteria.Since = time.Now().Add(-dur)
		}
		if strings.TrimSpace(matchers.AgeWindow.Min) != "" {
			dur, err := config.ParseRelativeDuration(matchers.AgeWindow.Min)
			if err != nil {
				return nil, fmt.Errorf("invalid age_window.min: %w", err)
			}
			criteria.Before = time.Now().Add(-dur)
		}
	}

	if len(matchers.SenderSubstring) > 0 {
		senderCriteria := make([]imap.SearchCriteria, 0, len(matchers.SenderSubstring))
		for _, value := range matchers.SenderSubstring {
			if strings.TrimSpace(value) == "" {
				continue
			}
			senderCriteria = append(senderCriteria, imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{{
					Key:   "From",
					Value: value,
				}},
			})
		}
		if combined := combineAnd(senderCriteria); combined != nil {
			criteria.And(combined)
		}
	}

	if len(matchers.Recipients) > 0 {
		recipientCriteria := make([]imap.SearchCriteria, 0, len(matchers.Recipients))
		for _, value := range matchers.Recipients {
			if strings.TrimSpace(value) == "" {
				continue
			}
			to := imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{{
					Key:   "To",
					Value: value,
				}},
			}
			cc := imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{{
					Key:   "Cc",
					Value: value,
				}},
			}
			recipientCriteria = append(recipientCriteria, imap.SearchCriteria{
				Or: [][2]imap.SearchCriteria{{to, cc}},
			})
		}
		if combined := combineAnd(recipientCriteria); combined != nil {
			criteria.And(combined)
		}
	}

	if len(matchers.CcSubstring) > 0 {
		ccCriteria := make([]imap.SearchCriteria, 0, len(matchers.CcSubstring))
		for _, value := range matchers.CcSubstring {
			if strings.TrimSpace(value) == "" {
				continue
			}
			ccCriteria = append(ccCriteria, imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{{
					Key:   "Cc",
					Value: value,
				}},
			})
		}
		if combined := combineAnd(ccCriteria); combined != nil {
			criteria.And(combined)
		}
	}

	if len(matchers.ReturnPathSubstring) > 0 {
		returnPathCriteria := make([]imap.SearchCriteria, 0, len(matchers.ReturnPathSubstring))
		for _, value := range matchers.ReturnPathSubstring {
			if strings.TrimSpace(value) == "" {
				continue
			}
			returnPathCriteria = append(returnPathCriteria, imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{{
					Key:   "Return-Path",
					Value: value,
				}},
			})
		}
		if combined := combineAnd(returnPathCriteria); combined != nil {
			criteria.And(combined)
		}
	}

	if len(matchers.BodySubstring) > 0 {
		bodyCriteria := make([]imap.SearchCriteria, 0, len(matchers.BodySubstring))
		for _, value := range matchers.BodySubstring {
			if strings.TrimSpace(value) == "" {
				continue
			}
			bodyCriteria = append(bodyCriteria, imap.SearchCriteria{
				Body: []string{value},
			})
		}
		if combined := combineAnd(bodyCriteria); combined != nil {
			criteria.And(combined)
		}
	}

	if len(matchers.ReplyToSubstring) > 0 {
		replyToCriteria := make([]imap.SearchCriteria, 0, len(matchers.ReplyToSubstring))
		for _, value := range matchers.ReplyToSubstring {
			if strings.TrimSpace(value) == "" {
				continue
			}
			replyToCriteria = append(replyToCriteria, imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{{
					Key:   "Reply-To",
					Value: value,
				}},
			})
		}
		if combined := combineAnd(replyToCriteria); combined != nil {
			criteria.And(combined)
		}
	}

	if len(matchers.ListIDSubstring) > 0 {
		listIDCriteria := make([]imap.SearchCriteria, 0, len(matchers.ListIDSubstring))
		for _, value := range matchers.ListIDSubstring {
			if strings.TrimSpace(value) == "" {
				continue
			}
			listIDCriteria = append(listIDCriteria, imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{{
					Key:   "List-ID",
					Value: value,
				}},
			})
		}
		if combined := combineAnd(listIDCriteria); combined != nil {
			criteria.And(combined)
		}
	}

	if matchers.Seen != nil {
		if *matchers.Seen {
			criteria.Flag = append(criteria.Flag, imap.FlagSeen)
		} else {
			criteria.NotFlag = append(criteria.NotFlag, imap.FlagSeen)
		}
	}
	if matchers.ListUnsubscribe != nil {
		listUnsubscribeCriteria := &imap.SearchCriteria{
			Header: []imap.SearchCriteriaHeaderField{{
				Key:   "List-Unsubscribe",
				Value: "",
			}},
		}
		if *matchers.ListUnsubscribe {
			criteria.And(listUnsubscribeCriteria)
		} else {
			criteria.Not = append(criteria.Not, *listUnsubscribeCriteria)
		}
	}

	return criteria, nil
}

func combineAnd(criteria []imap.SearchCriteria) *imap.SearchCriteria {
	if len(criteria) == 0 {
		return nil
	}
	combined := criteria[0]
	for i := 1; i < len(criteria); i++ {
		combined.And(&criteria[i])
	}
	return &combined
}

// SearchUIDsNewerThan returns UIDs greater than the provided last UID in the selected mailbox.
func (m *IMAPSearchManager) SearchUIDsNewerThan(ctx context.Context, lastUID uint32) ([]uint32, error) {
	if m.provider == nil {
		return nil, errors.New("IMAP client is not connected")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start := imap.UID(lastUID + 1)
	var uidSet imap.UIDSet
	uidSet.AddRange(start, 0)
	criteria := &imap.SearchCriteria{
		UID: []imap.UIDSet{uidSet},
	}
	data, err := m.provider().UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, err
	}
	uids := data.AllUIDs()
	out := make([]uint32, 0, len(uids))
	for _, uid := range uids {
		out = append(out, uint32(uid))
	}
	return out, nil
}
