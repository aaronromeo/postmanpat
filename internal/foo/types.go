package foo

import "time"

type MailData struct {
	UID                    uint32
	ReplyToDomains         []string
	SenderDomains          []string
	From                   []string
	ReturnPathDomain       string
	Recipients             []string
	Cc                     []string
	RecipientTags          []string
	Body                   string
	ListID                 string
	ListUnsubscribe        bool
	ListUnsubscribeTargets string
	PrecedenceRaw          string
	PrecedenceCategory     string
	XMailer                string
	UserAgent              string
	SubjectRaw             string
	SubjectNormalized      string
	MessageDate            time.Time
}
