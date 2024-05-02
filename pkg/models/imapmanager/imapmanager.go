package imapmanager

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"aaronromeo.com/postmanpat/pkg/base"
	"aaronromeo.com/postmanpat/pkg/models/mailbox"
	"aaronromeo.com/postmanpat/pkg/utils"
	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
	"github.com/pkg/errors"
)

type ImapManager interface {
	GetMailboxes() (map[string]base.SerializedMailbox, error)
	UnserializeMailboxes() (map[string]base.SerializedMailbox, error)
}

type ImapManagerImpl struct {
	client   base.Client
	logger   *slog.Logger
	username string
	password string
	ctx      context.Context
}

type ImapManagerOption func(*ImapManagerImpl) error

func NewImapManager(opts ...ImapManagerOption) (*ImapManagerImpl, error) {
	var imapMgr ImapManagerImpl
	for _, opt := range opts {
		err := opt(&imapMgr)
		if err != nil {
			return nil, err
		}
	}

	if imapMgr.username == "" {
		return nil, errors.New("requires username")
	}

	if imapMgr.username == "" {
		return nil, errors.New("requires password")
	}

	if imapMgr.client == nil {
		return nil, errors.New("requires client")
	}

	if imapMgr.logger == nil {
		return nil, errors.New("requires slogger")
	}

	return &imapMgr, nil
}

func WithTLSConfig(addr string, tlsConfig *tls.Config) ImapManagerOption {
	return func(imapMgr *ImapManagerImpl) error {
		c, err := imapclient.DialTLS(os.Getenv("IMAP_URL"), nil)
		if err != nil {
			return err
		}
		imapMgr.client = c
		return nil
	}
}

func WithAuth(username string, password string) ImapManagerOption {
	return func(imapMgr *ImapManagerImpl) error {
		imapMgr.username = username
		imapMgr.password = password
		return nil
	}
}

func WithClient(c base.Client) ImapManagerOption {
	return func(imapMgr *ImapManagerImpl) error {
		imapMgr.client = c
		return nil
	}
}

func WithLogger(logger *slog.Logger) ImapManagerOption {
	// slog.New(slog.NewJSONHandler(os.Stdout, nil))
	return func(isi *ImapManagerImpl) error {
		isi.logger = logger
		return nil
	}
}

func WithCtx(ctx context.Context) ImapManagerOption {
	// ctx := context.Background()
	return func(isi *ImapManagerImpl) error {
		isi.ctx = ctx
		return nil
	}
}

// Login
func (srv ImapManagerImpl) Login() error {
	if err := srv.client.Login(srv.username, srv.password); err != nil {
		srv.logger.ErrorContext(srv.ctx, fmt.Sprintf("Failed to login: %v", err), slog.Any("error", utils.WrapError(err)))
		return err
	}

	srv.logger.Info("Login success")

	return nil
}

// Logout
func (srv ImapManagerImpl) LogoutFn() func() {
	return func() {
		if err := srv.client.Logout(); err != nil {
			srv.logger.ErrorContext(srv.ctx, fmt.Sprintf("Failed to logout: %v", err), slog.Any("error", utils.WrapError(err)))
		}
	}
}

// GetMailboxes exports mailboxes from the server to the file system
func (srv ImapManagerImpl) GetMailboxes() (map[string]*mailbox.MailboxImpl, error) {
	defer srv.LogoutFn()()

	if err := srv.Login(); err != nil {
		srv.logger.ErrorContext(srv.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
		return nil, err
	}

	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() {
		done <- srv.client.List("", "*", mailboxes)
	}()

	verifiedMailboxObjs := map[string]*mailbox.MailboxImpl{}
	serializedMailboxObjs, err := srv.unserializeMailboxes()
	if err != nil {
		srv.logger.ErrorContext(srv.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
		return nil, err
	}
	srv.logger.Info("Retrieved serializedMailboxObjs")

	for m := range mailboxes {
		srv.logger.Info(fmt.Sprintf("Mailbox: %s", m.Name))
		if _, ok := serializedMailboxObjs[m.Name]; !ok {
			verifiedMailboxObjs[m.Name], err = mailbox.NewMailbox(
				mailbox.WithClient(srv.client),
				mailbox.WithLogger(srv.logger),
				mailbox.WithCtx(srv.ctx),
				mailbox.WithLoginFn(srv.Login),
				mailbox.WithLogoutFn(srv.client.Logout),
			)

			if err != nil {
				srv.logger.ErrorContext(srv.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
				return nil, err
			}

			verifiedMailboxObjs[m.Name].Name = m.Name
			verifiedMailboxObjs[m.Name].Deletable = false
			verifiedMailboxObjs[m.Name].Exportable = false
		} else {
			verifiedMailboxObjs[m.Name] = serializedMailboxObjs[m.Name]
		}
	}

	if err := <-done; err != nil {
		srv.logger.ErrorContext(srv.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
		return nil, err
	}

	return verifiedMailboxObjs, err
}

// unserializeMailboxes reads the mailbox list from the file system and returns a map of mailbox objects
func (srv ImapManagerImpl) unserializeMailboxes() (map[string]*mailbox.MailboxImpl, error) {
	serializedMailboxObjs := map[string]base.SerializedMailbox{}
	mailboxObjs := map[string]*mailbox.MailboxImpl{}

	if _, err := os.Stat(base.MailboxListFile); os.IsNotExist(err) {
		return mailboxObjs, nil
	}

	if mailboxFile, err := os.ReadFile(base.MailboxListFile); err != nil {
		srv.logger.ErrorContext(srv.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
		return nil, err
	} else {
		if err := json.Unmarshal(mailboxFile, &serializedMailboxObjs); err != nil {
			srv.logger.ErrorContext(srv.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
			return nil, err
		}
	}

	for name, serializedMailbox := range serializedMailboxObjs {
		mb, err := mailbox.NewMailbox(
			mailbox.WithClient(srv.client),
			mailbox.WithLogger(srv.logger),
			mailbox.WithCtx(srv.ctx),
			mailbox.WithLoginFn(srv.Login),
			mailbox.WithLogoutFn(srv.client.Logout),
		)
		if err != nil {
			srv.logger.ErrorContext(srv.ctx, err.Error(), slog.Any("error", utils.WrapError(err)))
			return nil, err
		}

		mb.Name = name
		mb.Deletable = serializedMailbox.Delete
		mb.Exportable = serializedMailbox.Export
		mb.Lifespan = serializedMailbox.Lifespan

		mailboxObjs[name] = mb

	}

	return mailboxObjs, nil
}

// Internal functions
