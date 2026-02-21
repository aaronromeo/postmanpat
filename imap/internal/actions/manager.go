package actions

import (
	"context"
	"errors"
	"strings"

	"github.com/emersion/go-imap/v2"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
)

type Actions interface {
	MoveByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, destination string) error
	DeleteByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, expunge bool) error
	MoveUIDs(ctx context.Context, uids []uint32, destination string) error
	DeleteUIDs(ctx context.Context, uids []uint32, expunge bool) error
}

// Interface to initialize the manager
type ClientProvider interface {
	IMAPClient() *giimapclient.Client
}

type IMAPActionManager struct {
	provider func() *giimapclient.Client
}

func New(provider ClientProvider) *IMAPActionManager {
	return &IMAPActionManager{provider: provider.IMAPClient}
}

// MoveUIDs move messages to a different destination folder.
func (c *IMAPActionManager) MoveUIDs(ctx context.Context, uids []uint32, destination string) error {
	if c.provider == nil || c.provider() == nil {
		return errors.New("IMAP client is not connected")
	}
	if len(uids) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(destination) == "" {
		return errors.New("destination mailbox is required")
	}

	var uidSet imap.UIDSet
	for _, uid := range uids {
		uidSet.AddNum(imap.UID(uid))
	}

	if _, err := c.provider().Move(uidSet, destination).Wait(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// MoveByMailbox moves messages for each mailbox to a destination folder.
func (c *IMAPActionManager) MoveByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, destination string) error {
	if c.provider == nil || c.provider() == nil {
		return errors.New("IMAP client is not connected")
	}
	if len(uidsByMailbox) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(destination) == "" {
		return errors.New("destination mailbox is required")
	}

	for mailbox, uids := range uidsByMailbox {
		mailbox = strings.TrimSpace(mailbox)
		if mailbox == "" {
			return errors.New("mailbox is required")
		}
		if _, err := c.provider().Select(mailbox, nil).Wait(); err != nil {
			return err
		}
		if err := c.MoveUIDs(ctx, uids, destination); err != nil {
			return err
		}
	}
	return nil
}

// DeleteUIDs marks messages as deleted and optionally expunges them.
func (c *IMAPActionManager) DeleteUIDs(ctx context.Context, uids []uint32, expunge bool) error {
	if c.provider == nil || c.provider() == nil {
		return errors.New("IMAP client is not connected")
	}
	if len(uids) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	var uidSet imap.UIDSet
	for _, uid := range uids {
		uidSet.AddNum(imap.UID(uid))
	}

	store := imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagDeleted},
	}
	if err := c.provider().Store(uidSet, &store, nil).Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if !expunge {
		return nil
	}
	if c.provider().Caps().Has(imap.CapUIDPlus) {
		_, err := c.provider().UIDExpunge(uidSet).Collect()
		return err
	}

	_, err := c.provider().Expunge().Collect()
	return err
}

// DeleteByMailbox marks messages as deleted and optionally expunges them per mailbox.
func (c *IMAPActionManager) DeleteByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, expunge bool) error {
	if c.provider == nil || c.provider() == nil {
		return errors.New("IMAP client is not connected")
	}
	if len(uidsByMailbox) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	for mailbox, uids := range uidsByMailbox {
		mailbox = strings.TrimSpace(mailbox)
		if mailbox == "" {
			return errors.New("mailbox is required")
		}
		if _, err := c.provider().Select(mailbox, nil).Wait(); err != nil {
			return err
		}
		if err := c.DeleteUIDs(ctx, uids, expunge); err != nil {
			return err
		}
	}
	return nil
}
