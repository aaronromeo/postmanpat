package imap

import (
	"crypto/tls"

	"github.com/aaronromeo/postmanpat/imap/internal/actions"
	foo "github.com/aaronromeo/postmanpat/imap/internal/maildata"
	"github.com/aaronromeo/postmanpat/imap/internal/searches"
	"github.com/aaronromeo/postmanpat/imap/internal/selectors"
	"github.com/aaronromeo/postmanpat/imap/internal/sessionmgr"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
)

type MailData = foo.MailData
type Option = sessionmgr.Option

func WithAddr(addr string) Option {
	return sessionmgr.WithAddr(addr)
}

func WithCreds(username, password string) Option {
	return sessionmgr.WithCreds(username, password)
}

func WithTLSConfig(config *tls.Config) Option {
	return sessionmgr.WithTLSConfig(config)
}

func WithUnilateralDataHandler(handler *giimapclient.UnilateralDataHandler) Option {
	return sessionmgr.WithUnilateralDataHandler(handler)
}

// Client encapsulates an IMAP connection for search operations.
type Client struct {
	*sessionmgr.IMAPConnector
	*searches.IMAPSearchManager
	*actions.IMAPActionManager
	*selectors.IMAPSelectorManager
}

func NewServer(opts ...Option) *Client {
	session := sessionmgr.NewServerConnector(opts...)
	return &Client{
		IMAPConnector:       session,
		IMAPSearchManager:   searches.New(session),
		IMAPActionManager:   actions.New(session),
		IMAPSelectorManager: selectors.New(session),
	}
}

func NewWatch(opts ...Option) *Client {
	session := sessionmgr.NewClientConnector(opts...)
	return &Client{
		IMAPConnector:       session,
		IMAPSearchManager:   searches.New(session),
		IMAPActionManager:   actions.New(session),
		IMAPSelectorManager: selectors.New(session),
	}
}
