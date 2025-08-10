package mailbox

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"aaronromeo.com/postmanpat/pkg/base"
	"aaronromeo.com/postmanpat/pkg/utils"
	"github.com/emersion/go-imap"
	_ "github.com/emersion/go-message/charset"
	"github.com/pkg/errors"
)

type MailboxImpl struct {
	base.SerializedMailbox

	Client      base.Client
	Ctx         context.Context
	FileManager utils.FileManager
	Logger      *slog.Logger
	LoginFn     func() (base.Client, error)
	LogoutFn    func() error
}

type MailboxOption func(*MailboxImpl) error

type OutputFileName struct {
	Name           string
	Differentiator string
	Extension      string
}

// NewMailbox creates a new Mailbox interface implementation.
// This function returns the interface type for better abstraction and testability.
func NewMailbox(opts ...MailboxOption) (Mailbox, error) {
	return NewMailboxImpl(opts...)
}

// NewMailboxImpl creates a new MailboxImpl concrete type.
// Use this when you specifically need the concrete implementation.
func NewMailboxImpl(opts ...MailboxOption) (*MailboxImpl, error) {
	var mb MailboxImpl
	for _, opt := range opts {
		err := opt(&mb)
		if err != nil {
			return nil, err
		}
	}

	if mb.Client == nil {
		return nil, errors.New("requires client")
	}

	if mb.Logger == nil {
		return nil, errors.New("requires slogger")
	}

	if mb.LoginFn == nil {
		return nil, errors.New("requires login function")
	}

	if mb.LogoutFn == nil {
		return nil, errors.New("requires logout function")
	}

	if mb.FileManager == nil {
		return nil, errors.New("requires file manager")
	}

	return &mb, nil
}

func WithClient(c base.Client) MailboxOption {
	return func(mb *MailboxImpl) error {
		mb.Client = c
		return nil
	}
}

func WithLogger(logger *slog.Logger) MailboxOption {
	return func(mb *MailboxImpl) error {
		mb.Logger = logger
		return nil
	}
}

func WithCtx(ctx context.Context) MailboxOption {
	return func(mb *MailboxImpl) error {
		mb.Ctx = ctx
		return nil
	}
}

func WithLoginFn(loginFn func() (base.Client, error)) MailboxOption {
	return func(mb *MailboxImpl) error {
		mb.LoginFn = loginFn
		return nil
	}
}

func WithLogoutFn(logoutFn func() error) MailboxOption {
	return func(mb *MailboxImpl) error {
		mb.LogoutFn = logoutFn
		return nil
	}
}

func WithFileManager(fileManager utils.FileManager) MailboxOption {
	return func(mb *MailboxImpl) error {
		mb.FileManager = fileManager
		return nil
	}
}

func (mb *MailboxImpl) Reap() error {
	return nil
}

func (mb *MailboxImpl) wrappedLogoutFn() func() {
	return func() {
		if err := mb.LogoutFn(); err != nil {
			mb.Logger.ErrorContext(mb.Ctx, fmt.Sprintf("Failed to logout: %v", err), slog.Any("error", utils.WrapError(err)))
		}
	}
}

func (mb *MailboxImpl) ProcessMailbox(ctx context.Context) error {
	if mb.Logger == nil {
		return fmt.Errorf("logger is nil")
	}

	switch {
	case mb.Exportable && mb.Deletable:
		mb.Logger.InfoContext(ctx, "Exporting and deleting mailbox", slog.String("name", mb.Name))
		err := mb.ExportAndDeleteMessages()
		if err != nil {
			return err
		}
	case mb.Deletable:
		mb.Logger.InfoContext(ctx, "Deleting mailbox", slog.String("name", mb.Name))
		err := mb.DeleteMessages()
		if err != nil {
			return err
		}
	case mb.Exportable: // This would be where we would have !mb.Deletable
		return errors.New("exportable but not deletable is not implemented")
	default:
		// fmt.Println("fmt.Prinln Skipping mailbox", mb.Name)
		// fmt.Printf("fmt.Prinln %v\n", ctx)
		// fmt.Printf("fmt.Prinln %v\n", mb.Logger)
		mb.Logger.InfoContext(ctx, "Skipping mailbox", slog.String("name", mb.Name))
	}
	return nil
}

func (mb *MailboxImpl) ExportAndDeleteMessages() error {
	// Defer logout
	defer mb.wrappedLogoutFn()

	if !mb.Exportable {
		return fmt.Errorf("mailbox %s is not exportable", mb.Name)
	}

	if !mb.Deletable {
		return fmt.Errorf("mailbox %s is not deletable", mb.Name)
	}

	// Login
	c, err := mb.LoginFn()
	if err != nil {
		mb.Logger.ErrorContext(mb.Ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
		return err
	}
	mb.Client = c

	messages, seqSet, err := mb.fetchMessages()
	if err != nil {
		return err
	}

	// Export messages
	err = mb.exportMessages(messages)
	if err != nil {
		return err
	}

	// Call the delete helper
	mb.deleteMessages(c, seqSet)

	return nil
}

func (mb *MailboxImpl) DeleteMessages() error {
	// Defer logout
	defer mb.wrappedLogoutFn()

	if !mb.Deletable {
		return fmt.Errorf("mailbox %s is not deletable", mb.Name)
	}

	// Login
	c, err := mb.LoginFn()
	if err != nil {
		mb.Logger.ErrorContext(mb.Ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
		return err
	}
	mb.Client = c

	_, seqSet, err := mb.fetchMessages()
	if err != nil {
		return err
	}

	// Call the delete helper
	mb.deleteMessages(c, seqSet)

	return nil
}

func (mb *MailboxImpl) Serialize() (base.SerializedMailbox, error) {
	return base.SerializedMailbox{
		Name:       mb.Name,
		Exportable: mb.Exportable,
		Deletable:  mb.Deletable,
		Lifespan:   mb.Lifespan,
	}, nil
}

func (mb *MailboxImpl) deleteMessages(c base.Client, seqSet *imap.SeqSet) {
	// First mark the message as deleted
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.DeletedFlag}
	if err := c.Store(seqSet, item, flags, nil); err != nil {
		log.Fatal(err)
	}

	// Then delete it
	if err := mb.Client.Expunge(nil); err != nil {
		log.Fatal(err)
	}
}

func (mb *MailboxImpl) fetchMessages() (chan *imap.Message, *imap.SeqSet, error) {
	// Select mailbox
	mbox, err := mb.Client.Select(mb.Name, false)
	if err != nil {
		mb.Logger.ErrorContext(mb.Ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
		return nil, nil, err
	}
	mb.Logger.Info(mb.Name, "Mailbox messages", mbox.Messages)

	// Set search criteria
	criteria := imap.NewSearchCriteria()
	criteria.Before = time.Now().Add(time.Hour * 24 * time.Duration(mb.Lifespan))
	ids, err := mb.Client.Search(criteria)
	if err != nil {
		log.Fatal(err)
	}

	seqSet := new(imap.SeqSet)
	if len(ids) <= 0 {
		return nil, seqSet, nil
	}
	seqSet.AddNum(ids...)

	section := imap.BodySectionName{}
	messages := make(chan *imap.Message, mbox.Messages)
	done := make(chan error, 1)
	go func() {
		done <- mb.Client.Fetch(seqSet, []imap.FetchItem{section.FetchItem(), imap.FetchEnvelope}, messages)
	}()

	mb.Logger.Info(mb.Name, "Fetched messages count", len(messages))

	return messages, seqSet, nil
}

func (mb *MailboxImpl) exportMessages(messages chan *imap.Message) error {
	for msg := range messages {
		metadata := CreateExportedEmailMetadata(msg, mb.Name)
		metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			mb.Logger.Error("Failed to serialize metadata", slog.Any("error", err))
			return err
		}
		baseFolder := filepath.Join(".", "exportedemails")
		basePath := filepath.Join(baseFolder, sanitize(mb.Name))

		// Unique folder for each email
		msgHash, err := json.Marshal(metadata)
		if err != nil {
			mb.Logger.Error("Unable to hash message", slog.Any("error", err))
			return err
		}
		emailFolderName := fmt.Sprintf("%s-%s-%x", metadata.Timestamp.Format("20060102T150405Z"), sanitize(metadata.Subject), md5.Sum([]byte(msgHash)))
		emailFolderPath := filepath.Join(basePath, emailFolderName)
		err = mb.FileManager.MkdirAll(emailFolderPath, os.ModePerm)
		if err != nil {
			mb.Logger.Error("Failed to create email folder", slog.Any("error", err))
			return err
		}

		metadataFile := filepath.Join(emailFolderPath, "metadata.json")

		err = mb.FileManager.WriteFile(metadataFile, metadataBytes, os.ModePerm)
		if err != nil {
			mb.Logger.Error("Failed to write metadata file", slog.Any("error", err))
			return err
		}

		mb.Logger.Info(mb.Name, "Subject", msg.Envelope.Subject)
		messageContainers, err := ExportedEmailContainerFactory(mb.Name, msg)
		if err != nil {
			mb.Logger.ErrorContext(mb.Ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
			return err
		}

		for _, emb := range messageContainers {
			err := emb.WriteToFile(mb.Logger, mb.FileManager, emailFolderPath)
			if err != nil {
				mb.Logger.ErrorContext(mb.Ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
				return err
			}
		}

		mb.Logger.Info(mb.Name, "Exported message", msg.Envelope.Subject)
	}
	return nil
}
