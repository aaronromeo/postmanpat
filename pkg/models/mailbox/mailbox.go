package mailbox

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"aaronromeo.com/postmanpat/pkg/base"
	"aaronromeo.com/postmanpat/pkg/utils"
	"github.com/emersion/go-imap"
	_ "github.com/emersion/go-message/charset"
	"github.com/pkg/errors"
)

type Mailbox interface {
	Reap() error
	ExportMessages() error
	DeleteMessages() error
	Serialize() (base.SerializedMailbox, error)
}

type MailboxImpl struct {
	Name        string `json:"name"`
	Deletable   bool   `json:"delete"`
	Exportable  bool   `json:"export"`
	Lifespan    int    `json:"lifespan"`
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

func NewMailbox(opts ...MailboxOption) (*MailboxImpl, error) {
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
	// slog.New(slog.NewJSONHandler(os.Stdout, nil))
	return func(mb *MailboxImpl) error {
		mb.Logger = logger
		return nil
	}
}

func WithCtx(ctx context.Context) MailboxOption {
	// ctx := context.Background()
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

func (mb *MailboxImpl) ExportMessages() error {
	// Defer logout
	defer mb.wrappedLogoutFn()

	// Login
	c, err := mb.LoginFn()
	if err != nil {
		mb.Logger.ErrorContext(mb.Ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
		return err
	}
	mb.Client = c

	// Select mailbox
	mbox, err := mb.Client.Select(mb.Name, false)
	if err != nil {
		mb.Logger.ErrorContext(mb.Ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
		return err
	}
	mb.Logger.Info(mb.Name, "Mailbox messages", mbox.Messages)

	seqSet := new(imap.SeqSet)
	seqSet.AddRange(1, mbox.Messages)

	section := imap.BodySectionName{}
	messages := make(chan *imap.Message, mbox.Messages)
	done := make(chan error, 1)
	go func() {
		done <- mb.Client.Fetch(seqSet, []imap.FetchItem{section.FetchItem(), imap.FetchEnvelope}, messages)
	}()

	mb.Logger.Info(mb.Name, "Fetched messages count", len(messages))

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
