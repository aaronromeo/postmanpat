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
	"strings"
	"time"

	"aaronromeo.com/postmanpat/pkg/base"
	"aaronromeo.com/postmanpat/pkg/utils"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
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
	loginFn  func() (base.Client, error)
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

func WithLoginFn(loginFn func() (base.Client, error)) MailboxOption {
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
	c, err := mb.loginFn()
	if err != nil {
		mb.logger.ErrorContext(mb.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
		return err
	}
	mb.client = c

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
			switch {
			case errors.Is(err, io.EOF):
				return nil
			case err != nil && strings.Contains(err.Error(), "multipart: NextPart: EOF"):
				return nil
			case err == nil:
				messageEntity := *p
				header := messageEntity.Header
				t, params, err := header.ContentType()
				if err != nil {
					mb.logger.ErrorContext(mb.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
					return err
				}
				b, err := io.ReadAll((*p).Body)

				if len(b) == 0 { // Skip empty parts
					continue
				}

				if err != nil {
					mb.logger.ErrorContext(mb.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
					return err
				}

				var outfile *os.File
				switch t {
				case "text/html":
					outfile, err = os.Create(fmt.Sprintf("%s_%d.html", filename, partCount))

				case "text/plain":
					outfile, err = os.Create(fmt.Sprintf("%s_%d.txt", filename, partCount))

				case "application/msword":
					outfile, err = os.Create(fmt.Sprintf("%s_%d.doc", filename, partCount))

				case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
					outfile, err = os.Create(fmt.Sprintf("%s_%d.docx", filename, partCount))

				case "application/zip":
					outfile, err = os.Create(fmt.Sprintf("%s_%d.zip", filename, partCount))

				case "multipart/alternative":
					// Typically, you would not save "multipart/alternative" as a file because it's a container, not actual data.
					// It may be useful to log this case or handle embedded parts separately.
					mb.logger.InfoContext(mb.ctx, "Multipart/alternative encountered; processing embedded parts.")
					continue // Skip to the next part

				case "application/octet-stream":
					// Guessing the filename from parameters or defaulting to a generic bin file
					filenameParam := params["name"]
					if filenameParam == "" {
						filenameParam = fmt.Sprintf("octet-stream_%d.bin", partCount)
					}
					outfile, err = os.Create(fmt.Sprintf("%s_%s", filename, filenameParam))

				default:
					mb.logger.ErrorContext(mb.ctx, errors.New("Unknown header content type").Error(), slog.Any("type", t))
					outfile, err = os.Create(fmt.Sprintf("%s_%d_unknown", filename, partCount))
				}

				if err != nil {
					mb.logger.ErrorContext(mb.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
					return err
				}

				writer := bufio.NewWriter(outfile)
				_, err = writer.Write(b)
				if err != nil {
					mb.logger.ErrorContext(
						mb.ctx,
						err.Error(),
						slog.Any("error", utils.WrapError(err)),
						slog.Any("outfile", outfile),
						slog.Any("buffer", b),
					)
					return err
				}

				if err = writer.Flush(); err != nil {
					mb.logger.ErrorContext(mb.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
				}
				if err = outfile.Close(); err != nil {
					mb.logger.ErrorContext(mb.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
				}
			default:
				mb.logger.ErrorContext(mb.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
				return err
			}

		}
	} else {

		t, _, err := m.Header.ContentType()
		if err != nil {
			mb.logger.ErrorContext(mb.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
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
	err := mb.convertMessageToString(literal.(io.Reader), dest)
	if err != nil {
		mb.logger.ErrorContext(mb.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
		return err
	}
	// log.Printf("Saving email to %s with %s", dest, emailBodyContents)
	return nil // os.WriteFile(dest, []byte(emailBodyContents), 0644)
}
