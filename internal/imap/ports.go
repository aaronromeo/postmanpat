package imap

import (
	"github.com/aaronromeo/postmanpat/internal/imap/actions"
	"github.com/aaronromeo/postmanpat/internal/imap/searches"
	"github.com/aaronromeo/postmanpat/internal/imap/sessionmgr"
)

type ServerRunner interface {
	sessionmgr.ServerConnector
	searches.ServerSearcher
	actions.Actions
}
