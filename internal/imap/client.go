package imap

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	netmail "net/mail"
	"regexp"
	"sort"
	"strings"

	"github.com/aaronromeo/postmanpat/internal/foo"
	"github.com/aaronromeo/postmanpat/internal/imap/actionmanager"
	"github.com/aaronromeo/postmanpat/internal/imap/session_manager"
	"github.com/emersion/go-imap/v2"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
	"github.com/emersion/go-message/textproto"
)

// Client encapsulates an IMAP connection for search operations.
type Client struct {
	*session_manager.IMAPConnector
	*actionmanager.IMAPManager
}

func New(opts ...session_manager.Option) *Client {
	session := session_manager.NewServerConnector(opts...)
	manager := actionmanager.New(session)
	client := &Client{
		session,
		manager,
	}
	return client
}

// FetchSenderData returns sender data for the provided UIDs.
func (c *Client) FetchSenderData(ctx context.Context, uids []uint32) ([]foo.MailData, error) {
	if c.IMAPClient() == nil {
		return nil, errors.New("IMAP client is not connected")
	}
	if len(uids) == 0 {
		return []foo.MailData{}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var uidSet imap.UIDSet
	for _, uid := range uids {
		uidSet.AddNum(imap.UID(uid))
	}

	headerSection := &imap.FetchItemBodySection{
		Specifier:    imap.PartSpecifierHeader,
		HeaderFields: []string{"List-ID", "List-Unsubscribe", "Precedence", "X-Mailer", "User-Agent", "Reply-To", "Return-Path"},
		Peek:         true,
	}
	bodySection := &imap.FetchItemBodySection{
		Specifier: imap.PartSpecifierText,
		Peek:      true,
	}
	fetchOptions := &imap.FetchOptions{
		Envelope:    true,
		UID:         true,
		BodySection: []*imap.FetchItemBodySection{headerSection, bodySection},
	}

	fetchCmd := c.IMAPClient().Fetch(uidSet, fetchOptions)
	rows := make([]foo.MailData, 0, len(uids))
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
		var body string
		var uid uint32
		for {
			item := msg.Next()
			if item == nil {
				break
			}
			if data, ok := item.(giimapclient.FetchItemDataEnvelope); ok {
				envelope = data.Envelope
				continue
			}
			if data, ok := item.(giimapclient.FetchItemDataUID); ok {
				uid = uint32(data.UID)
				continue
			}
			if data, ok := item.(giimapclient.FetchItemDataBodySection); ok {
				if data.Literal == nil {
					continue
				}
				if data.MatchCommand(headerSection) {
					parsedHeader, err := readHeader(data.Literal)
					if err == nil {
						header = parsedHeader
					}
					continue
				}
				if data.MatchCommand(bodySection) {
					raw, err := io.ReadAll(data.Literal)
					if err == nil {
						body = string(raw)
					}
					continue
				}
				_, _ = io.Copy(io.Discard, data.Literal)
			}
		}
		if envelope == nil {
			continue
		}

		replyToHosts := parseReplyToDomains(header)

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
		ccRecipients := []string{}
		for _, addr := range envelope.Cc {
			ccRecipients = append(ccRecipients, addr.Addr())
		}
		from := []string{}
		for _, addr := range envelope.From {
			from = append(from, addr.Addr())
		}

		data := foo.MailData{
			UID:               uid,
			ReplyToDomains:    replyToHosts,
			From:              from,
			SenderDomains:     fromHosts,
			ReturnPathDomain:  parseReturnPathDomain(header),
			Recipients:        recipients,
			Cc:                ccRecipients,
			RecipientTags:     recipientTags,
			Body:              body,
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
func (c *Client) FetchSenderDataByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32) (map[string][]foo.MailData, error) {
	if c.IMAPClient() == nil {
		return nil, errors.New("IMAP client is not connected")
	}
	if len(uidsByMailbox) == 0 {
		return map[string][]foo.MailData{}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	results := make(map[string][]foo.MailData)
	for mailbox, uids := range uidsByMailbox {
		mailbox = strings.TrimSpace(mailbox)
		if mailbox == "" {
			return nil, errors.New("mailbox is required")
		}
		if _, err := c.IMAPClient().Select(mailbox, nil).Wait(); err != nil {
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

// SearchUIDsNewerThan returns UIDs greater than the provided last UID in the selected mailbox.
func (c *Client) SearchUIDsNewerThan(ctx context.Context, lastUID uint32) ([]uint32, error) {
	if c.IMAPClient() == nil {
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
	data, err := c.IMAPClient().UIDSearch(criteria, nil).Wait()
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

// SelectMailbox selects a mailbox and returns its metadata.
func (c *Client) SelectMailbox(ctx context.Context, mailbox string) (*imap.SelectData, error) {
	if c.IMAPClient() == nil {
		return nil, errors.New("IMAP client is not connected")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(mailbox) == "" {
		return nil, errors.New("mailbox is required")
	}
	return c.IMAPClient().Select(mailbox, nil).Wait()
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

func parseReplyToDomains(header *mail.Header) []string {
	raw := headerText(header, "Reply-To")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	addresses, err := netmail.ParseAddressList(raw)
	if err != nil {
		return nil
	}
	hosts := make(map[string]struct{})
	for _, addr := range addresses {
		parts := strings.Split(addr.Address, "@")
		if len(parts) != 2 {
			continue
		}
		host := strings.ToLower(strings.TrimSpace(parts[1]))
		if host == "" {
			continue
		}
		hosts[host] = struct{}{}
	}
	if len(hosts) == 0 {
		return nil
	}
	sorted := make([]string, 0, len(hosts))
	for host := range hosts {
		sorted = append(sorted, host)
	}
	sort.Strings(sorted)
	return sorted
}

func parseReturnPathDomain(header *mail.Header) string {
	raw := headerText(header, "Return-Path")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.TrimPrefix(raw, "<")
	raw = strings.TrimSuffix(raw, ">")
	parts := strings.Split(raw, "@")
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parts[1]))
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
	if c.IMAPClient() == nil {
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

	if _, err := c.IMAPClient().Move(uidSet, destination).Wait(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// MoveByMailbox moves messages for each mailbox to a destination folder.
func (c *Client) MoveByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, destination string) error {
	if c.IMAPClient() == nil {
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
		if _, err := c.IMAPClient().Select(mailbox, nil).Wait(); err != nil {
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
	if c.IMAPClient() == nil {
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
	if err := c.IMAPClient().Store(uidSet, &store, nil).Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if !expunge {
		return nil
	}
	if c.IMAPClient().Caps().Has(imap.CapUIDPlus) {
		_, err := c.IMAPClient().UIDExpunge(uidSet).Collect()
		return err
	}

	_, err := c.IMAPClient().Expunge().Collect()
	return err
}

// DeleteByMailbox marks messages as deleted and optionally expunges them per mailbox.
func (c *Client) DeleteByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, expunge bool) error {
	if c.IMAPClient() == nil {
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
		if _, err := c.IMAPClient().Select(mailbox, nil).Wait(); err != nil {
			return err
		}
		if err := c.DeleteUIDs(ctx, uids, expunge); err != nil {
			return err
		}
	}
	return nil
}
