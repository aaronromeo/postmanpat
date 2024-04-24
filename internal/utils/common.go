package utils

import (
	"encoding/json"
	"log"
	"os"

	"github.com/emersion/go-imap"
)

// GetMailboxes exports mailboxes from the server to the file system
func GetMailboxes(c Client) map[string]Mailbox {
	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.List("", "*", mailboxes)
	}()

	verifiedMailboxObjs := map[string]Mailbox{}
	serializedMailboxObjs, err := UnserializeMailboxes()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Mailboxes:")
	for m := range mailboxes {
		log.Println("* " + m.Name)
		if _, ok := serializedMailboxObjs[m.Name]; !ok {
			verifiedMailboxObjs[m.Name] = Mailbox{Name: m.Name, Delete: false, Export: false}
		} else {
			verifiedMailboxObjs[m.Name] = serializedMailboxObjs[m.Name]
		}
	}

	if err := <-done; err != nil {
		log.Fatal(err)
	}

	return verifiedMailboxObjs
}

// UnserializeMailboxes reads the mailbox list from the file system and returns a map of mailbox objects
func UnserializeMailboxes() (map[string]Mailbox, error) {
	mailboxObjs := map[string]Mailbox{}

	if _, err := os.Stat(MailboxListFile); os.IsNotExist(err) {
		return mailboxObjs, nil
	}

	if mailboxFile, err := os.ReadFile(MailboxListFile); err != nil {
		return nil, err
	} else {
		if err := json.Unmarshal(mailboxFile, &mailboxObjs); err != nil {
			return nil, err
		}
	}
	return mailboxObjs, nil
}
