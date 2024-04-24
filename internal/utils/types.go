package utils

import "github.com/emersion/go-imap"

type Mailbox struct {
	Name     string `json:"name"`
	Delete   bool   `json:"delete"`
	Export   bool   `json:"export"`
	Lifespan int    `json:"lifespan"`
}

// Client is an interface to abstract the client.Client methods used
type Client interface {
	List(ref, name string, ch chan *imap.MailboxInfo) error
}
