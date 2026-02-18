package imap

import (
	"context"

	"github.com/aaronromeo/postmanpat/internal/imap/actionmanager"
	"github.com/aaronromeo/postmanpat/internal/imap/session_manager"
)

type Actions interface {
	MoveByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, destination string) error
	DeleteByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, expunge bool) error
}

type ServerRunner interface {
	session_manager.ServerConnector
	actionmanager.ServerSearcher
	Actions
}
