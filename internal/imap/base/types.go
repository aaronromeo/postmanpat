package base

import (
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
)

type State struct {
	Client *giimapclient.Client
}
