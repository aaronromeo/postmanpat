package base

import (
	"github.com/emersion/go-imap"
)

type SerializedMailbox struct {
	Name       string `json:"name"`
	Deletable  bool   `json:"delete"`
	Exportable bool   `json:"export"`
	Lifespan   int    `json:"lifespan"`
}

// Client is an interface to abstract the client.Client methods used
type Client interface {
	Expunge(ch chan uint32) error
	Fetch(seqset *imap.SeqSet, items []imap.FetchItem, ch chan *imap.Message) error
	List(ref, name string, ch chan *imap.MailboxInfo) error
	Login(username string, password string) error
	Logout() error
	Search(criteria *imap.SearchCriteria) (seqNums []uint32, err error)
	Select(name string, readOnly bool) (*imap.MailboxStatus, error)
	State() imap.ConnState
	Store(seqset *imap.SeqSet, item imap.StoreItem, value interface{}, ch chan *imap.Message) error
}

type Service interface {
}
