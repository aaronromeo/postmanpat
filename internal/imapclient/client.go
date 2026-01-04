package imapclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// Client encapsulates an IMAP connection for search operations.
type Client struct {
	Addr      string
	Username  string
	Password  string
	Mailbox   string
	TLSConfig *tls.Config

	client *imapclient.Client
}

type MailData struct {
	ReplyToDomains string
	SenderDomains  string
	Recipients     string
	Count          int
}

// Connect establishes the IMAP connection, logs in, and selects the mailbox.
func (c *Client) Connect() error {
	if strings.TrimSpace(c.Addr) == "" {
		return errors.New("IMAP address is required")
	}
	if strings.TrimSpace(c.Username) == "" || strings.TrimSpace(c.Password) == "" {
		return errors.New("IMAP credentials are required")
	}
	if strings.TrimSpace(c.Mailbox) == "" {
		c.Mailbox = "INBOX"
	}

	var options *imapclient.Options
	if c.TLSConfig != nil {
		options = &imapclient.Options{TLSConfig: c.TLSConfig}
	}

	client, err := imapclient.DialTLS(c.Addr, options)
	if err != nil {
		return err
	}

	if err := client.Login(c.Username, c.Password).Wait(); err != nil {
		_ = client.Logout().Wait()
		return err
	}

	if _, err := client.Select(c.Mailbox, nil).Wait(); err != nil {
		_ = client.Logout().Wait()
		return err
	}

	c.client = client
	return nil
}

// Close logs out and clears the connection.
func (c *Client) Close() error {
	if c.client == nil {
		return nil
	}
	err := c.client.Logout().Wait()
	c.client = nil
	return err
}

// SearchByMatchers returns UIDs for messages matching the provided matchers via IMAP SEARCH.
func (c *Client) SearchByMatchers(ctx context.Context, matchers config.Matchers) ([]uint32, error) {
	if c.client == nil {
		return nil, errors.New("IMAP client is not connected")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	criteria := &imap.SearchCriteria{}

	if matchers.AgeDays != nil && *matchers.AgeDays > 0 {
		criteria.Before = time.Now().AddDate(0, 0, -*matchers.AgeDays)
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
		if combined := combineOr(senderCriteria); combined != nil {
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
		if combined := combineOr(recipientCriteria); combined != nil {
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
		if combined := combineOr(bodyCriteria); combined != nil {
			criteria.And(combined)
		}
	}

	data, err := c.client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	uids := data.AllUIDs()
	matches := make([]uint32, 0, len(uids))
	for _, uid := range uids {
		matches = append(matches, uint32(uid))
	}
	return matches, nil
}

// FetchSenderData returns unique sender domains for the provided UIDs.
func (c *Client) FetchSenderData(ctx context.Context, uids []uint32) (map[string]MailData, error) {
	if c.client == nil {
		return nil, errors.New("IMAP client is not connected")
	}
	if len(uids) == 0 {
		return map[string]MailData{}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var uidSet imap.UIDSet
	for _, uid := range uids {
		uidSet.AddNum(imap.UID(uid))
	}

	fetchOptions := &imap.FetchOptions{
		Envelope: true,
		UID:      true,
	}

	fetchCmd := c.client.Fetch(uidSet, fetchOptions)
	domains := map[string]MailData{}
	for {
		if err := ctx.Err(); err != nil {
			_ = fetchCmd.Close()
			return nil, err
		}

		msg := fetchCmd.Next()
		if msg == nil {
			break
		}

		var envelope *imap.Envelope
		for {
			item := msg.Next()
			if item == nil {
				break
			}
			if data, ok := item.(imapclient.FetchItemDataEnvelope); ok {
				envelope = data.Envelope
			}
		}
		if envelope == nil {
			continue
		}

		replyToHosts := []string{}
		for _, addr := range envelope.ReplyTo {
			host := strings.ToLower(strings.TrimSpace(addr.Host))
			if host == "" {
				continue
			}
			replyToHosts = append(replyToHosts, host)
		}

		fromHosts := []string{}
		for _, addr := range envelope.From {
			host := strings.ToLower(strings.TrimSpace(addr.Host))
			if host == "" {
				continue
			}
			fromHosts = append(fromHosts, host)
		}

		recipients := []string{}
		for _, addr := range envelope.To {
			recipients = append(recipients, addr.Addr())
		}

		data := MailData{
			ReplyToDomains: strings.Join(replyToHosts, ","),
			SenderDomains:  strings.Join(fromHosts, ","),
			Recipients:     strings.Join(recipients, ","),
		}

		key := fmt.Sprintf("%v", data)
		if value, ok := domains[key]; !ok {
			data.Count = 1
		} else {
			data.Count = value.Count + 1
		}
		domains[key] = data
	}

	if err := fetchCmd.Close(); err != nil {
		return nil, err
	}

	return domains, nil
}

// DeleteUIDs marks messages as deleted and expunges them.
func (c *Client) DeleteUIDs(ctx context.Context, uids []uint32) error {
	if c.client == nil {
		return errors.New("IMAP client is not connected")
	}
	if len(uids) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	var uidSet imap.UIDSet
	for _, uid := range uids {
		uidSet.AddNum(imap.UID(uid))
	}

	store := imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagDeleted},
	}
	if err := c.client.Store(uidSet, &store, nil).Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if c.client.Caps().Has(imap.CapUIDPlus) {
		_, err := c.client.UIDExpunge(uidSet).Collect()
		return err
	}

	_, err := c.client.Expunge().Collect()
	return err
}

func combineOr(criteria []imap.SearchCriteria) *imap.SearchCriteria {
	if len(criteria) == 0 {
		return nil
	}
	combined := criteria[0]
	for i := 1; i < len(criteria); i++ {
		combined = imap.SearchCriteria{
			Or: [][2]imap.SearchCriteria{{combined, criteria[i]}},
		}
	}
	return &combined
}
