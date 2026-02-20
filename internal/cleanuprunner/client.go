package cleanuprunner

import (
	"github.com/aaronromeo/postmanpat/internal/imap"
	"github.com/aaronromeo/postmanpat/internal/imap/actions"
	"github.com/aaronromeo/postmanpat/internal/imap/searches"
	"github.com/aaronromeo/postmanpat/internal/imap/selectors"
	"github.com/aaronromeo/postmanpat/internal/imap/sessionmgr"
)

type ServerRunner interface {
	sessionmgr.ServerConnector
	searches.ServerSearcher
	actions.Actions
}

func New(opts ...sessionmgr.Option) *imap.Client {
	session := sessionmgr.NewServerConnector(opts...)
	client := &imap.Client{
		IMAPConnector:       session,
		IMAPSearchManager:   searches.New(session),
		IMAPActionManager:   actions.New(session),
		IMAPSelectorManager: selectors.New(session),
	}
	return client
}
