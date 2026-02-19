package watchrunner

import (
	"github.com/aaronromeo/postmanpat/internal/imap/actions"
	"github.com/aaronromeo/postmanpat/internal/imap/searches"
	"github.com/aaronromeo/postmanpat/internal/imap/selectors"
	"github.com/aaronromeo/postmanpat/internal/imap/sessionmgr"
)

type WatchRunner interface {
	sessionmgr.ClientConnector
	selectors.ClientSelectors
	searches.ClientSearcher
	actions.Actions
}
