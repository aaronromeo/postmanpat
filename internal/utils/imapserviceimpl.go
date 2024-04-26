package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/emersion/go-imap"
)

type ImapService interface {
	GetMailboxes() (map[string]Mailbox, error)
	UnserializeMailboxes() (map[string]Mailbox, error)
}

type ImapServiceImpl struct {
	client Client
	logger *slog.Logger
	ctx    context.Context
}

type ImapServiceOption func(*ImapServiceImpl)

func NewImapService(opts ...ImapServiceOption) (*ImapServiceImpl, error) {
	var imapService ImapServiceImpl
	for _, opt := range opts {
		opt(&imapService)
	}

	if imapService.client == nil {
		return nil, errors.New("requires client")
	}

	if imapService.logger == nil {
		return nil, errors.New("requires slogger")
	}

	if imapService.ctx == nil {
		return nil, errors.New("requires ctx")
	}

	return &imapService, nil
}

func WithClient(c Client) ImapServiceOption {
	return func(imapService *ImapServiceImpl) {
		imapService.client = c
	}
}

func WithLogger(logger *slog.Logger) ImapServiceOption {
	// slog.New(slog.NewJSONHandler(os.Stdout, nil))
	return func(isi *ImapServiceImpl) {
		isi.logger = logger
	}
}

func WithCtx(ctx context.Context) ImapServiceOption {
	// ctx := context.Background()
	return func(isi *ImapServiceImpl) {
		isi.ctx = ctx
	}
}

// GetMailboxes exports mailboxes from the server to the file system
func (srv ImapServiceImpl) GetMailboxes() (map[string]Mailbox, error) {
	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() {
		done <- srv.client.List("", "*", mailboxes)
	}()

	verifiedMailboxObjs := map[string]Mailbox{}
	serializedMailboxObjs, err := srv.UnserializeMailboxes()
	if err != nil {
		srv.logger.ErrorContext(srv.ctx, err.Error(), slog.Any("error", err))
		return nil, err
	}

	for m := range mailboxes {
		srv.logger.Info(fmt.Sprintf("Mailbox: %s", m.Name))
		if _, ok := serializedMailboxObjs[m.Name]; !ok {
			verifiedMailboxObjs[m.Name] = Mailbox{Name: m.Name, Delete: false, Export: false}
		} else {
			verifiedMailboxObjs[m.Name] = serializedMailboxObjs[m.Name]
		}
	}

	if err := <-done; err != nil {
		srv.logger.ErrorContext(srv.ctx, err.Error(), slog.Any("error", err))
		return nil, err
	}

	return verifiedMailboxObjs, err
}

// UnserializeMailboxes reads the mailbox list from the file system and returns a map of mailbox objects
func (srv ImapServiceImpl) UnserializeMailboxes() (map[string]Mailbox, error) {
	mailboxObjs := map[string]Mailbox{}

	if _, err := os.Stat(MailboxListFile); os.IsNotExist(err) {
		return mailboxObjs, nil
	}

	if mailboxFile, err := os.ReadFile(MailboxListFile); err != nil {
		srv.logger.ErrorContext(srv.ctx, err.Error(), slog.Any("error", err))
		return nil, err
	} else {
		if err := json.Unmarshal(mailboxFile, &mailboxObjs); err != nil {
			srv.logger.ErrorContext(srv.ctx, err.Error(), slog.Any("error", err))
			return nil, err
		}
	}
	return mailboxObjs, nil
}
