package mailbox

import (
	"context"

	"aaronromeo.com/postmanpat/pkg/base"
)

type Mailbox interface {
	Reap() error
	ExportAndDeleteMessages() error
	DeleteMessages() error
	Serialize() (base.SerializedMailbox, error)
	ProcessMailbox(ctx context.Context) error
}
