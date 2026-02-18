package base

import (
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
)

type EmersionInterface interface {
	Logout() *giimapclient.Command
}

type State struct {
	Client *giimapclient.Client
}
