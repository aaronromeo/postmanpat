package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	message "github.com/emersion/go-message"
	"github.com/pkg/errors"
)

func ExportMailboxes(c *client.Client) map[string]Mailbox {
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

func ExportEmails(c *client.Client, mailbox string) {
	// Select mailbox
	mbox, err := c.Select(mailbox, false)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Mailbox %s has %d messages", mailbox, mbox.Messages)

	// criteria := imap.NewSearchCriteria()
	// uids, err := c.Search(criteria)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	seqSet := new(imap.SeqSet)
	seqSet.AddRange(1, mbox.Messages)

	// messages := make(chan *imap.Message, 10)
	// done := make(chan error, 1)
	// go func() {
	// 	done <- c.Fetch(seqSet, []imap.FetchItem{imap.FetchEnvelope}, messages)
	// }()

	// seqSet := new(imap.SeqSet)
	// seqSet.AddNum(uids...)
	section := imap.BodySectionName{}
	// items := []imap.FetchItem{section.FetchItem(), imap.FetchEnvelope}
	messages := make(chan *imap.Message, mbox.Messages)
	done := make(chan error, 1)
	go func() {
		// if err := c.Fetch(seqSet, items, messages); err != nil {
		// 	log.Fatal(err)
		// }
		done <- c.Fetch(seqSet, []imap.FetchItem{section.FetchItem(), imap.FetchEnvelope}, messages)
	}()

	log.Printf("Fetched %d messages", len(messages))

	for msg := range messages {
		fmt.Println("Subject:", msg.Envelope.Subject)
		for _, literal := range msg.Body {
			saveEmail(mailbox, msg.Envelope.Date, literal)
		}
	}
}

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

func convertMessageToString(r io.Reader, filename string) error { //(string, error) {
	m, err := message.Read(r)
	if message.IsUnknownCharset(err) {
		// This error is not fatal
		return errors.New(fmt.Sprintf("Unknown encoding: %v", err))
	} else if err != nil {
		return err
	}

	if mr := m.MultipartReader(); mr != nil {
		// This is a multipart message
		log.Println("This is a multipart message containing:")
		partCount := 1
		for {
			p, err := mr.NextPart()
			switch err {
			case io.EOF:
				// No more parts
			case nil:
				messageEntity := *p
				header := messageEntity.Header
				t, params, err := header.ContentType()
				if err != nil {
					return err
				}
				log.Println("A part with type ", t, params)
				// log.Printf("Inline text: %v", string(b))
				b, _ := io.ReadAll((*p).Body)
				var outfile *os.File
				switch t {
				case "text/html":
					outfile, err = os.Create(fmt.Sprintf("%s_%d.html", filename, partCount))
					if err != nil {
						return err
					}
				case "text/plain":
					// This is the message's text (can be plain-text or HTML)
					outfile, err = os.Create(fmt.Sprintf("%s_%d.txt", filename, partCount))
					if err != nil {
						return err
					}
				case "application/octet-stream":
					// This is an attachment
					// filename, _ := htype.Filename()
					// log.Printf("Attachment: %v", filename)
					// b, _ := io.ReadAll((*p).Body)
					// log.Printf("Inline text: %v", string(b))
					// log.Printf("Inline text: %v", string(b))
					outfile, err = os.Create(fmt.Sprintf("%s_%d_%s", filename, partCount, params["name"]))
					if err != nil {
						return err
					}
				}
				log.Println("Before writing to file")
				writer := bufio.NewWriter(outfile)
				writer.WriteString(string(b))
				log.Println("After writing to file")
			default:
				return err
			}

		}
	} else {

		t, _, _ := m.Header.ContentType()
		log.Println("This is a non-multipart message with type", t)
	}

	return nil
}

func saveEmail(mailbox string, timestamp time.Time, literal interface{}) error {
	// Save email to disk
	timestampStr := timestamp.Format("20060102150405")
	fileName := fmt.Sprintf("%s-%s.eml", mailbox, timestampStr)

	var re = regexp.MustCompile(`[^a-zA-Z0-9]`)
	dest := filepath.Join(filepath.Base("."), "exportedemails", re.ReplaceAllString(fileName, "_"))
	// emailBodyContents,
	error := convertMessageToString(literal.(io.Reader), dest)
	if error != nil {
		return error
	}
	// log.Printf("Saving email to %s with %s", dest, emailBodyContents)
	return nil // os.WriteFile(dest, []byte(emailBodyContents), 0644)
}
