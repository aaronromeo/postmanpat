package serverrunner

import (
	"context"

	appconfig "github.com/aaronromeo/postmanpat/envmgr"
	"github.com/aaronromeo/postmanpat/imap"
)

type ServerRunner interface {
	Connect() error
	Close() error
	SearchByServerMatchers(ctx context.Context, matchers appconfig.ServerMatchers) (map[string][]uint32, error)
	MoveByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, destination string) error
	DeleteByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, expunge bool) error
}

func New(opts ...imap.Option) *imap.Client {
	return imap.NewServer(opts...)
}
