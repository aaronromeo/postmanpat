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
	client      base.Client
	dialTLS     func(address string, tlsConfig *tls.Config) (base.Client, error)
	username    string
	password    string
	address     string
	logger      *slog.Logger
	tlsConfig   *tls.Config
	ctx         context.Context
	fileCreator utils.FileWriter
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

	if imapMgr.dialTLS == nil {
		imapMgr.dialTLS = func(address string, tlsConfig *tls.Config) (base.Client, error) {
			c, err := imapclient.DialTLS(address, tlsConfig)
			if err != nil {
				return nil, err
			}
			return c, nil
		}
	}

	if imapMgr.username == "" {
		return nil, errors.New("requires username")
	}

	if imapMgr.password == "" {
		return nil, errors.New("requires password")
	}

	if imapMgr.client == nil && imapMgr.address == "" {
		return nil, errors.New("requires client or address")
	}

	if imapMgr.client == nil {
		c, err := imapMgr.dialTLS(imapMgr.address, imapMgr.tlsConfig)
		if err != nil {
			return nil, err
		}
		imapMgr.client = c
	}

	if imapMgr.logger == nil {
		return nil, errors.New("requires slogger")
	}

	if imapMgr.fileCreator == nil {
		return nil, errors.New("requires file creator")
	}

	return &imapMgr, nil
}

func WithTLSConfig(addr string, tlsConfig *tls.Config) ImapManagerOption {
	return func(imapMgr *ImapManagerImpl) error {
		imapMgr.address = addr
		imapMgr.tlsConfig = tlsConfig
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

func WithDialTLS(d func(address string, tlsConfig *tls.Config) (base.Client, error)) ImapManagerOption {
	return func(imapMgr *ImapManagerImpl) error {
		imapMgr.dialTLS = d
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

func WithFileManager(fileCreator utils.FileWriter) ImapManagerOption {
	return func(isi *ImapManagerImpl) error {
		isi.fileCreator = fileCreator
		return nil
	}
}

// Login
func (srv ImapManagerImpl) Login() (base.Client, error) {
	state := srv.client.State()
	switch state {
	case imap.NotAuthenticatedState:
		if err := srv.client.Login(srv.username, srv.password); err != nil {
			srv.logger.ErrorContext(srv.ctx, fmt.Sprintf("Failed to login: %v", err), slog.Any("error", utils.WrapError(err)))
			return srv.client, err
		}
		srv.logger.Info("Login success")
	case imap.AuthenticatedState:
		srv.logger.Info("Already authenticated")
	case imap.SelectedState:
		srv.logger.Info("Already selected mailbox")
	default: // imap.LogoutState and imap.ConnectedState
		c, err := srv.dialTLS(srv.address, srv.tlsConfig)
		if err != nil {
			srv.logger.ErrorContext(srv.ctx, fmt.Sprintf("Failed to create a client: %v", err), slog.Any("error", utils.WrapError(err)))
			return srv.client, err
		}
		srv.client = c
		srv.logger.Info("Login success")

		if err := srv.client.Login(srv.username, srv.password); err != nil {
			srv.logger.ErrorContext(srv.ctx, fmt.Sprintf("Failed to login: %v", err), slog.Any("error", utils.WrapError(err)))
			return srv.client, err
		}
		srv.logger.Info("Login success")
	}

	return srv.client, nil
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

	if _, err := srv.Login(); err != nil {
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
				mailbox.WithFileManager(utils.OSFileWriter{}),
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
			mailbox.WithFileManager(utils.OSFileWriter{}),
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
