package selectors

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
	"github.com/emersion/go-imap/v2"
	giimap "github.com/emersion/go-imap/v2"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
	"github.com/emersion/go-message/textproto"
)

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

type ClientSelectors interface {
	SelectMailbox(ctx context.Context, mailbox string) (*giimap.SelectData, error)
	FetchSenderData(ctx context.Context, uids []uint32) ([]foo.MailData, error)
}

// Interface to initialize the manager
type ClientProvider interface {
	IMAPClient() *giimapclient.Client
}

type IMAPSelectorManager struct {
	provider func() *giimapclient.Client
}

func New(provider ClientProvider) *IMAPSelectorManager {
	return &IMAPSelectorManager{provider: provider.IMAPClient}
}

// SelectMailbox selects a mailbox and returns its metadata.
func (c *IMAPSelectorManager) SelectMailbox(ctx context.Context, mailbox string) (*imap.SelectData, error) {
	if c.provider == nil || c.provider() == nil {
		return nil, errors.New("IMAP client is not connected")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(mailbox) == "" {
		return nil, errors.New("mailbox is required")
	}
	return c.provider().Select(mailbox, nil).Wait()
}

// FetchSenderDataByMailbox returns sender data per mailbox for the provided UIDs.
func (c *IMAPSelectorManager) FetchSenderDataByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32) (map[string][]foo.MailData, error) {
	if c.provider == nil || c.provider() == nil {
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
		if _, err := c.provider().Select(mailbox, nil).Wait(); err != nil {
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

// FetchSenderData returns sender data for the provided UIDs.
func (c *IMAPSelectorManager) FetchSenderData(ctx context.Context, uids []uint32) ([]foo.MailData, error) {
	if c.provider == nil || c.provider() == nil {
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

	fetchCmd := c.provider().Fetch(uidSet, fetchOptions)
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

func recipientTag(recipient string) string {
	return recipientTagPattern.ReplaceAllString(strings.ToLower(recipient), "_")
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
