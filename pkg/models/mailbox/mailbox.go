package mailbox

import (
	"context"
	"log/slog"

	"aaronromeo.com/postmanpat/pkg/base"
	"github.com/pkg/errors"
)

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
