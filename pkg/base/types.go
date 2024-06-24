package base

import (
	"github.com/emersion/go-imap"
)

type SerializedMailbox struct {
	Name     string `json:"name"`
	Delete   bool   `json:"delete"`
	Export   bool   `json:"export"`
	Lifespan int    `json:"lifespan"`
}

// Client is an interface to abstract the client.Client methods used
type Client interface {
	Fetch(seqset *imap.SeqSet, items []imap.FetchItem, ch chan *imap.Message) error
	List(ref, name string, ch chan *imap.MailboxInfo) error
	Login(username string, password string) error
	Logout() error
	Select(name string, readOnly bool) (*imap.MailboxStatus, error)
	State() imap.ConnState
}

type Service interface {
}
