package utils

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
	"github.com/pkg/errors"
)

type ImapService interface {
	GetMailboxes() (map[string]Mailbox, error)
	UnserializeMailboxes() (map[string]Mailbox, error)
}

type ImapServiceImpl struct {
	client   Client
	logger   *slog.Logger
	username string
	password string
	ctx      context.Context
}

type ImapServiceOption func(*ImapServiceImpl) error

func NewImapService(opts ...ImapServiceOption) (*ImapServiceImpl, error) {
	var imapService ImapServiceImpl
	for _, opt := range opts {
		err := opt(&imapService)
		if err != nil {
			return nil, err
		}
	}

	// requiredFields := []interface{}{
	// 	imapService.username,
	// 	imapService.password,
	// 	imapService.client,
	// 	imapService.logger,
	// 	imapService.ctx,
	// }

	// for _, field := range requiredFields {
	// 	fieldName := reflect.TypeOf(field).Name() // TODO: This is wrong
	// 	switch reflect.TypeOf(field).Kind() {
	// 	case reflect.String:
	// 		if field == "" {
	// 			return nil, fmt.Errorf("requires %s", fieldName)
	// 		}
	// 	case reflect.Ptr:
	// 		if field == nil {
	// 			return nil, fmt.Errorf("requires %s", fieldName)
	// 		}
	// 	}
	// }

	if imapService.username == "" {
		return nil, errors.New("requires username")
	}

	if imapService.username == "" {
		return nil, errors.New("requires password")
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

func WithTLSConfig(addr string, tlsConfig *tls.Config) ImapServiceOption {
	return func(imapService *ImapServiceImpl) error {
		c, err := imapclient.DialTLS(os.Getenv("IMAP_URL"), nil)
		if err != nil {
			return err
		}
		imapService.client = c
		return nil
	}
}

func WithAuth(username string, password string) ImapServiceOption {
	return func(imapService *ImapServiceImpl) error {
		imapService.username = username
		imapService.password = password
		return nil
	}
}

func WithClient(c Client) ImapServiceOption {
	return func(imapService *ImapServiceImpl) error {
		imapService.client = c
		return nil
	}
}

func WithLogger(logger *slog.Logger) ImapServiceOption {
	// slog.New(slog.NewJSONHandler(os.Stdout, nil))
	return func(isi *ImapServiceImpl) error {
		isi.logger = logger
		return nil
	}
}

func WithCtx(ctx context.Context) ImapServiceOption {
	// ctx := context.Background()
	return func(isi *ImapServiceImpl) error {
		isi.ctx = ctx
		return nil
	}
}

// Login
func (srv ImapServiceImpl) Login() error {
	if err := srv.client.Login(srv.username, srv.password); err != nil {
		srv.logger.ErrorContext(srv.ctx, fmt.Sprintf("Failed to login: %v", err), slog.Any("error", wrapError(err)))
		return err
	}

	srv.logger.Info("Login success")

	return nil
}

// Logout
func (srv ImapServiceImpl) LogoutFn() func() error {
	return func() error {
		if err := srv.client.Logout(); err != nil {
			srv.logger.ErrorContext(srv.ctx, fmt.Sprintf("Failed to logout: %v", err), slog.Any("error", wrapError(err)))
			return err
		}

		return nil
	}
}

// GetMailboxes exports mailboxes from the server to the file system
func (srv ImapServiceImpl) GetMailboxes() (map[string]Mailbox, error) {
	defer srv.LogoutFn()()

	if err := srv.Login(); err != nil {
		return nil, err
	}

	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() {
		done <- srv.client.List("", "*", mailboxes)
	}()

	verifiedMailboxObjs := map[string]Mailbox{}
	serializedMailboxObjs, err := srv.UnserializeMailboxes()
	if err != nil {
		srv.logger.ErrorContext(srv.ctx, err.Error(), slog.Any("error", wrapError(err)))
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
		srv.logger.ErrorContext(srv.ctx, err.Error(), slog.Any("error", wrapError(err)))
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
		srv.logger.ErrorContext(srv.ctx, err.Error(), slog.Any("error", wrapError(err)))
		return nil, err
	} else {
		if err := json.Unmarshal(mailboxFile, &mailboxObjs); err != nil {
			srv.logger.ErrorContext(srv.ctx, err.Error(), slog.Any("error", wrapError(err)))
			return nil, err
		}
	}
	return mailboxObjs, nil
}

// Internal functions

func wrapError(err error) error {
	_, file, line, _ := runtime.Caller(1)
	return errors.New(fmt.Sprintf("error at %s:%d: %v", file, line, err))
}
