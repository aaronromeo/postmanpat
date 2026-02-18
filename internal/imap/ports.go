package imap

import (
	"context"

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/aaronromeo/postmanpat/internal/imap/auth"
)

type Searcher interface {
	SearchByServerMatchers(ctx context.Context, matchers config.ServerMatchers) (map[string][]uint32, error)
}

type Actions interface {
	MoveByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, destination string) error
	DeleteByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, expunge bool) error
}

type ServerRunner interface {
	auth.Connector
	Searcher
	Actions
}
