package mailbox

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"aaronromeo.com/postmanpat/pkg/base"
	"aaronromeo.com/postmanpat/pkg/utils"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-message"
	"github.com/pkg/errors"
)

const EMAIL_EXPORT_TIMESTAMP_FORMAT = "20060102150405"

type Mailbox interface {
	Reap() error
	ExportMessages() error
	DeleteMessages() error
	Serialize() (base.SerializedMailbox, error)
}

type MailboxImpl struct {
	Name       string `json:"name"`
	Deletable  bool   `json:"delete"`
	Exportable bool   `json:"export"`
	Lifespan   int    `json:"lifespan"`

	client   base.Client
	logger   *slog.Logger
	ctx      context.Context
	loginFn  func() error
	logoutFn func() error
}

type MailboxOption func(*MailboxImpl) error

func NewMailbox(opts ...MailboxOption) (*MailboxImpl, error) {
	var mb MailboxImpl
	for _, opt := range opts {
		err := opt(&mb)
		if err != nil {
			return nil, err
		}
	}

	if mb.client == nil {
		return nil, errors.New("requires client")
	}

	if mb.logger == nil {
		return nil, errors.New("requires slogger")
	}

	if mb.loginFn == nil {
		return nil, errors.New("requires login function")
	}

	if mb.logoutFn == nil {
		return nil, errors.New("requires logout function")
	}

	return &mb, nil
}

func WithClient(c base.Client) MailboxOption {
	return func(mb *MailboxImpl) error {
		mb.client = c
		return nil
	}
}

func WithLogger(logger *slog.Logger) MailboxOption {
	// slog.New(slog.NewJSONHandler(os.Stdout, nil))
	return func(mb *MailboxImpl) error {
		mb.logger = logger
		return nil
	}
}

func WithCtx(ctx context.Context) MailboxOption {
	// ctx := context.Background()
	return func(mb *MailboxImpl) error {
		mb.ctx = ctx
		return nil
	}
}

func WithLoginFn(loginFn func() error) MailboxOption {
	return func(mb *MailboxImpl) error {
		mb.loginFn = loginFn
		return nil
	}
}

func WithLogoutFn(logoutFn func() error) MailboxOption {
	return func(mb *MailboxImpl) error {
		mb.logoutFn = logoutFn
		return nil
	}
}

func (mb *MailboxImpl) Reap() error {
	return nil
}

func (mb *MailboxImpl) ExportMessages() error {
	// Defer logout
	defer mb.logoutFn()

	// Login
	if err := mb.loginFn(); err != nil {
		mb.logger.ErrorContext(mb.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
		return err
	}

	// Select mailbox
	mbox, err := mb.client.Select(mb.Name, false)
	if err != nil {
		mb.logger.ErrorContext(mb.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
		return err
	}
	mb.logger.Info(mb.Name, "Mailbox messages", mbox.Messages)

	seqSet := new(imap.SeqSet)
	seqSet.AddRange(1, mbox.Messages)

	section := imap.BodySectionName{}
	messages := make(chan *imap.Message, mbox.Messages)
	done := make(chan error, 1)
	go func() {
		done <- mb.client.Fetch(seqSet, []imap.FetchItem{section.FetchItem(), imap.FetchEnvelope}, messages)
	}()

	mb.logger.Info(mb.Name, "Fetched messages count", len(messages))

	for msg := range messages {
		mb.logger.Info(mb.Name, "Subject", msg.Envelope.Subject)
		for _, literal := range msg.Body {
			if err := mb.saveEmail(msg.Envelope.Date, literal); err != nil {
				mb.logger.ErrorContext(mb.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
				return err
			}
		}
	}

	return nil
}

func (mb *MailboxImpl) DeleteMessages() error {
	return nil
}

func (mb *MailboxImpl) Serialize() (base.SerializedMailbox, error) {
	return base.SerializedMailbox{
		Name:     mb.Name,
		Export:   mb.Exportable,
		Delete:   mb.Deletable,
		Lifespan: mb.Lifespan,
	}, nil
}

func (mb *MailboxImpl) convertMessageToString(r io.Reader, filename string) error { //(string, error) {
	m, err := message.Read(r)
	if message.IsUnknownCharset(err) {
		// This error is not fatal
		return errors.New(fmt.Sprintf("Unknown encoding: %v", err))
	} else if err != nil {
		return err
	}

	if mr := m.MultipartReader(); mr != nil {
		// This is a multipart message
		// mb.logger.Info(mb.Name, "multipart message")

		partCount := 1
		for {
			p, err := mr.NextPart()
			switch err {
			case io.EOF:
				// No more parts
			case nil:
				messageEntity := *p
				header := messageEntity.Header
				t, params, err := header.ContentType()
				if err != nil {
					return err
				}
				b, err := io.ReadAll((*p).Body)

				if len(b) == 0 { // Skip empty parts
					continue
				}

				if err != nil {
					return err
				}

				var outfile *os.File
				switch t {
				case "text/html":
					outfile, err = os.Create(fmt.Sprintf("%s_%d.html", filename, partCount))
					if err != nil {
						return err
					}
				case "text/plain":
					// This is the message's text (can be plain-text or HTML)
					outfile, err = os.Create(fmt.Sprintf("%s_%d.txt", filename, partCount))
					if err != nil {
						return err
					}
				case "application/octet-stream":
					outfile, err = os.Create(fmt.Sprintf("%s_%d_%s", filename, partCount, params["name"]))
					if err != nil {
						return err
					}
				}
				writer := bufio.NewWriter(outfile)
				_, err = writer.WriteString(string(b))
				if err != nil {
					return err
				}
			default:
				return err
			}

		}
	} else {

		t, _, err := m.Header.ContentType()
		if err != nil {
			return err
		}
		mb.logger.Info(mb.Name, "non-multipart message", t)
	}

	return nil
}

func (mb *MailboxImpl) saveEmail(timestamp time.Time, literal interface{}) error {
	// Save email to disk
	timestampStr := timestamp.Format(EMAIL_EXPORT_TIMESTAMP_FORMAT)
	fileName := fmt.Sprintf("%s-%s.eml", mb.Name, timestampStr)

	var re = regexp.MustCompile(`[^a-zA-Z0-9]`)
	dest := filepath.Join(filepath.Base("."), "exportedemails", re.ReplaceAllString(fileName, "_"))
	// emailBodyContents,
	error := mb.convertMessageToString(literal.(io.Reader), dest)
	if error != nil {
		return error
	}
	// log.Printf("Saving email to %s with %s", dest, emailBodyContents)
	return nil // os.WriteFile(dest, []byte(emailBodyContents), 0644)
}
