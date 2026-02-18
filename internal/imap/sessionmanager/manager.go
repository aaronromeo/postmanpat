package sessionmanager

import (
	"crypto/tls"
	"errors"
	"strings"

	"github.com/aaronromeo/postmanpat/internal/imap/base"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
)

type Option func(*IMAPConnector)

type ServerConnector interface {
	Connect() error
	Close() error

	IMAPClient() *giimapclient.Client
}

type ClientConnector interface {
	ServerConnector

	Idle() (*giimapclient.IdleCommand, error)
}

type IMAPConnector struct {
	Addr                  string
	Username              string
	Password              string
	TLSConfig             *tls.Config
	UnilateralDataHandler *giimapclient.UnilateralDataHandler

	base.State
}

func WithAddr(a string) Option {
	return func(c *IMAPConnector) {
		c.Addr = a
	}
}

func WithCreds(username string, password string) Option {
	return func(c *IMAPConnector) {
		c.Username = username
		c.Password = password
	}
}

func WithTLSConfig(config *tls.Config) Option {
	return func(state *IMAPConnector) {
		state.TLSConfig = config
	}
}

func WithUnilateralDataHandler(handler *giimapclient.UnilateralDataHandler) Option {
	return func(state *IMAPConnector) {
		state.UnilateralDataHandler = handler
	}
}

func NewServerConnector(opts ...Option) *IMAPConnector {
	c := &IMAPConnector{}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

func NewClientConnector(opts ...Option) *IMAPConnector {
	c := &IMAPConnector{}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *IMAPConnector) IMAPClient() *giimapclient.Client {
	return c.Client
}

// Connect establishes the IMAP connection, logs in, and selects the mailbox.
func (c *IMAPConnector) Connect() error {
	if err := validateDeps(c); err != nil {
		return err
	}

	var options *giimapclient.Options
	if c.TLSConfig != nil || c.UnilateralDataHandler != nil {
		options = &giimapclient.Options{
			TLSConfig:             c.TLSConfig,
			UnilateralDataHandler: c.UnilateralDataHandler,
		}
	}

	client, err := giimapclient.DialTLS(c.Addr, options)
	if err != nil {
		return err
	}

	if err := client.Login(c.Username, c.Password).Wait(); err != nil {
		_ = client.Logout().Wait()
		return err
	}

	c.Client = client
	return nil
}

// Idle starts an IMAP IDLE command.
func (c *IMAPConnector) Idle() (*giimapclient.IdleCommand, error) {
	if c.Client == nil {
		return nil, errors.New("IMAP client is not connected")
	}
	return c.Client.Idle()
}

// Close logs out and clears the connection.
func (c *IMAPConnector) Close() error {
	if c.Client == nil {
		return nil
	}
	err := c.Client.Logout().Wait()
	c.Client = nil
	return err
}

func validateDeps(state *IMAPConnector) error {
	if strings.TrimSpace(state.Addr) == "" {
		return errors.New("IMAP address is required")
	}
	if strings.TrimSpace(state.Username) == "" || strings.TrimSpace(state.Password) == "" {
		return errors.New("IMAP credentials are required")
	}

	return nil
}
