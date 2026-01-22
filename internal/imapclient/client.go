package imapclient

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
	"github.com/emersion/go-message/textproto"
)

// Client encapsulates an IMAP connection for search operations.
type Client struct {
	Addr      string
	Username  string
	Password  string
	TLSConfig *tls.Config

	client *imapclient.Client
}

type MailData struct {
	ReplyToDomains         []string
	SenderDomains          []string
	Recipients             []string
	RecipientTags          []string
	ListID                 string
	ListUnsubscribe        bool
	ListUnsubscribeTargets string
	PrecedenceRaw          string
	PrecedenceCategory     string
	XMailer                string
	UserAgent              string
	SubjectRaw             string
	SubjectNormalized      string
	MessageDate            time.Time
}

// Connect establishes the IMAP connection, logs in, and selects the mailbox.
func (c *Client) Connect() error {
	if strings.TrimSpace(c.Addr) == "" {
		return errors.New("IMAP address is required")
	}
	if strings.TrimSpace(c.Username) == "" || strings.TrimSpace(c.Password) == "" {
		return errors.New("IMAP credentials are required")
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
// Results are grouped by mailbox to avoid UID collisions across folders.
func (c *Client) SearchByMatchers(ctx context.Context, matchers config.Matchers) (map[string][]uint32, error) {
	if c.client == nil {
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

		if _, err := c.client.Select(folder, nil).Wait(); err != nil {
			return nil, err
		}

		criteria := buildSearchCriteria(matchers)

		data, err := c.client.UIDSearch(criteria, nil).Wait()
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

// FetchSenderData returns sender data for the provided UIDs.
func (c *Client) FetchSenderData(ctx context.Context, uids []uint32) ([]MailData, error) {
	if c.client == nil {
		return nil, errors.New("IMAP client is not connected")
	}
	if len(uids) == 0 {
		return []MailData{}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var uidSet imap.UIDSet
	for _, uid := range uids {
		uidSet.AddNum(imap.UID(uid))
	}

	bodySection := &imap.FetchItemBodySection{
		Specifier:    imap.PartSpecifierHeader,
		HeaderFields: []string{"List-ID", "List-Unsubscribe", "Precedence", "X-Mailer", "User-Agent"},
	}
	fetchOptions := &imap.FetchOptions{
		Envelope:    true,
		UID:         true,
		BodySection: []*imap.FetchItemBodySection{bodySection},
	}

	fetchCmd := c.client.Fetch(uidSet, fetchOptions)
	rows := make([]MailData, 0, len(uids))
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
		var header *mail.Header
		for {
			item := msg.Next()
			if item == nil {
				break
			}
			if data, ok := item.(imapclient.FetchItemDataEnvelope); ok {
				envelope = data.Envelope
				continue
			}
			if data, ok := item.(imapclient.FetchItemDataBodySection); ok {
				if data.Literal == nil {
					continue
				}
				if data.MatchCommand(bodySection) {
					parsedHeader, err := readHeader(data.Literal)
					if err == nil {
						header = parsedHeader
					}
					continue
				}
				_, _ = io.Copy(io.Discard, data.Literal)
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
		recipientTags := []string{}
		for _, addr := range envelope.To {
			recipients = append(recipients, addr.Addr())
			recipientTags = append(recipientTags, recipientTag(addr.Addr()))
		}

		data := MailData{
			ReplyToDomains:    replyToHosts,
			SenderDomains:     fromHosts,
			Recipients:        recipients,
			RecipientTags:     recipientTags,
			ListID:            headerText(header, "List-ID"),
			PrecedenceRaw:     headerText(header, "Precedence"),
			XMailer:           headerText(header, "X-Mailer"),
			UserAgent:         headerText(header, "User-Agent"),
			SubjectRaw:        strings.TrimSpace(envelope.Subject),
			SubjectNormalized: normalizeSubject(envelope.Subject),
			MessageDate:       envelope.Date,
		}

		listUnsubscribeTargets := parseListUnsubscribeTargets(header)
		data.ListUnsubscribe = len(listUnsubscribeTargets) > 0
		data.ListUnsubscribeTargets = strings.Join(listUnsubscribeTargets, ",")
		data.PrecedenceCategory = normalizePrecedence(data.PrecedenceRaw)

		rows = append(rows, data)
	}

	if err := fetchCmd.Close(); err != nil {
		return nil, err
	}

	return rows, nil
}

// FetchSenderDataByMailbox returns sender data per mailbox for the provided UIDs.
func (c *Client) FetchSenderDataByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32) (map[string][]MailData, error) {
	if c.client == nil {
		return nil, errors.New("IMAP client is not connected")
	}
	if len(uidsByMailbox) == 0 {
		return map[string][]MailData{}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	results := make(map[string][]MailData)
	for mailbox, uids := range uidsByMailbox {
		mailbox = strings.TrimSpace(mailbox)
		if mailbox == "" {
			return nil, errors.New("mailbox is required")
		}
		if _, err := c.client.Select(mailbox, nil).Wait(); err != nil {
			return nil, err
		}

		data, err := c.FetchSenderData(ctx, uids)
		if err != nil {
			return nil, err
		}
		results[mailbox] = data
	}

	return results, nil
}

func readHeader(literal imap.LiteralReader) (*mail.Header, error) {
	if literal == nil {
		return nil, errors.New("missing header literal")
	}
	raw, err := io.ReadAll(literal)
	if err != nil {
		return nil, err
	}
	tpHeader, err := textproto.ReadHeader(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		return nil, err
	}
	msgHeader := message.Header{Header: tpHeader}
	header := mail.Header{Header: msgHeader}
	return &header, nil
}

func headerText(header *mail.Header, key string) string {
	if header == nil {
		return ""
	}
	value, err := header.Text(key)
	if err != nil {
		return strings.TrimSpace(header.Get(key))
	}
	return strings.TrimSpace(value)
}

func parseListUnsubscribeTargets(header *mail.Header) []string {
	if header == nil {
		return nil
	}
	fields := header.FieldsByKey("List-Unsubscribe")
	if fields.Len() == 0 {
		return nil
	}

	targets := make(map[string]struct{})
	for fields.Next() {
		value, err := fields.Text()
		if err != nil {
			value = fields.Value()
		}
		for _, token := range strings.Split(value, ",") {
			target := strings.TrimSpace(token)
			target = strings.TrimPrefix(target, "<")
			target = strings.TrimSuffix(target, ">")
			if target == "" {
				continue
			}
			targets[target] = struct{}{}
		}
	}

	sorted := make([]string, 0, len(targets))
	for target := range targets {
		sorted = append(sorted, target)
	}
	sort.Strings(sorted)
	return sorted
}

func normalizePrecedence(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "bulk", "list":
		return normalized
	default:
		return ""
	}
}

var (
	subjectDatePatterns = []*regexp.Regexp{
		regexp.MustCompile(`\b\d{4}[-/]\d{1,2}[-/]\d{1,2}\b`),
		regexp.MustCompile(`\b\d{1,2}[-/]\d{1,2}[-/]\d{2,4}\b`),
	}
	subjectCounterPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\(\s*\d+\s*\)`),
		regexp.MustCompile(`#\d+`),
	}
	subjectMonthPattern  = regexp.MustCompile(`\b(?:jan|feb|mar|apr|may|jun|jul|aug|sep|sept|oct|nov|dec)[a-z]*\b`)
	subjectNumberPattern = regexp.MustCompile(`\d+`)

	recipientTagPattern = regexp.MustCompile(`[^a-z0-9]`)
)

func normalizeSubject(subject string) string {
	normalized := strings.TrimSpace(subject)
	if normalized == "" {
		return ""
	}
	normalized = strings.ToLower(normalized)
	for _, pattern := range subjectDatePatterns {
		normalized = pattern.ReplaceAllString(normalized, "")
	}
	normalized = subjectMonthPattern.ReplaceAllString(normalized, "{{mm}}")
	for _, pattern := range subjectCounterPatterns {
		normalized = pattern.ReplaceAllString(normalized, "")
	}
	normalized = subjectNumberPattern.ReplaceAllString(normalized, "{{n}}")
	normalized = strings.Join(strings.Fields(normalized), " ")
	return normalized
}

func recipientTag(recipient string) string {
	return recipientTagPattern.ReplaceAllString(strings.ToLower(recipient), "_")
}

// MoveUIDs move messages to a different destination folder.
func (c *Client) MoveUIDs(ctx context.Context, uids []uint32, destination string) error {
	if c.client == nil {
		return errors.New("IMAP client is not connected")
	}
	if len(uids) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(destination) == "" {
		return errors.New("destination mailbox is required")
	}

	var uidSet imap.UIDSet
	for _, uid := range uids {
		uidSet.AddNum(imap.UID(uid))
	}

	if _, err := c.client.Move(uidSet, destination).Wait(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// MoveByMailbox moves messages for each mailbox to a destination folder.
func (c *Client) MoveByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, destination string) error {
	if c.client == nil {
		return errors.New("IMAP client is not connected")
	}
	if len(uidsByMailbox) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(destination) == "" {
		return errors.New("destination mailbox is required")
	}

	for mailbox, uids := range uidsByMailbox {
		mailbox = strings.TrimSpace(mailbox)
		if mailbox == "" {
			return errors.New("mailbox is required")
		}
		if _, err := c.client.Select(mailbox, nil).Wait(); err != nil {
			return err
		}
		if err := c.MoveUIDs(ctx, uids, destination); err != nil {
			return err
		}
	}
	return nil
}

// DeleteUIDs marks messages as deleted and optionally expunges them.
func (c *Client) DeleteUIDs(ctx context.Context, uids []uint32, expunge bool) error {
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

	if !expunge {
		return nil
	}
	if c.client.Caps().Has(imap.CapUIDPlus) {
		_, err := c.client.UIDExpunge(uidSet).Collect()
		return err
	}

	_, err := c.client.Expunge().Collect()
	return err
}

// DeleteByMailbox marks messages as deleted and optionally expunges them per mailbox.
func (c *Client) DeleteByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, expunge bool) error {
	if c.client == nil {
		return errors.New("IMAP client is not connected")
	}
	if len(uidsByMailbox) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	for mailbox, uids := range uidsByMailbox {
		mailbox = strings.TrimSpace(mailbox)
		if mailbox == "" {
			return errors.New("mailbox is required")
		}
		if _, err := c.client.Select(mailbox, nil).Wait(); err != nil {
			return err
		}
		if err := c.DeleteUIDs(ctx, uids, expunge); err != nil {
			return err
		}
	}
	return nil
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

func buildSearchCriteria(matchers config.Matchers) *imap.SearchCriteria {
	criteria := &imap.SearchCriteria{}
	criteria.NotFlag = append(criteria.NotFlag, imap.FlagDeleted)

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

	return criteria
}
