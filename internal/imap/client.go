package imap

import (
	"github.com/aaronromeo/postmanpat/internal/imap/actions"
	"github.com/aaronromeo/postmanpat/internal/imap/searches"
	"github.com/aaronromeo/postmanpat/internal/imap/selectors"
	"github.com/aaronromeo/postmanpat/internal/imap/sessionmgr"
)

// Client encapsulates an IMAP connection for search operations.
type Client struct {
	*sessionmgr.IMAPConnector
	*searches.IMAPSearchManager
	*actions.IMAPActionManager
	*selectors.IMAPSelectorManager
}

func New(opts ...sessionmgr.Option) *Client {
	session := sessionmgr.NewServerConnector(opts...)
	client := &Client{
		session,
		searches.New(session),
		actions.New(session),
		selectors.New(session),
	}
	return client
}
