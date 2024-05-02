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
	List(ref, name string, ch chan *imap.MailboxInfo) error
	Select(name string, readOnly bool) (*imap.MailboxStatus, error)
	Logout() error
	Login(username string, password string) error
	Fetch(seqset *imap.SeqSet, items []imap.FetchItem, ch chan *imap.Message) error
	State() imap.ConnState
}

type Service interface {
}
