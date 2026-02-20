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
