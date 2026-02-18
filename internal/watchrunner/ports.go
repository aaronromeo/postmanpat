package watchrunner

import (
	"context"

	"github.com/aaronromeo/postmanpat/internal/foo"
	"github.com/aaronromeo/postmanpat/internal/imap/sessionmanager"
	giimap "github.com/emersion/go-imap/v2"
)

type Connector interface {
	sessionmanager.ClientConnector
}

type Selector interface {
	SelectMailbox(ctx context.Context, mailbox string) (*giimap.SelectData, error)
	FetchSenderData(ctx context.Context, uids []uint32) ([]foo.MailData, error)
}

type Searcher interface {
	SearchUIDsNewerThan(ctx context.Context, lastUID uint32) ([]uint32, error)
}

type Actions interface {
	MoveUIDs(ctx context.Context, uids []uint32, destination string) error
	DeleteUIDs(ctx context.Context, uids []uint32, expunge bool) error
}

type WatchRunner interface {
	Connector
	Selector
	Searcher
	Actions
}
