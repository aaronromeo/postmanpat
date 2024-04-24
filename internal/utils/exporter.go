package utils

import (
	"bufio"
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

// ExportEmailsFromMailbox exports emails from the specified mailbox to the file system
func ExportEmailsFromMailbox(c *client.Client, mailbox string) {
	// Select mailbox
	mbox, err := c.Select(mailbox, false)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Mailbox %s has %d messages", mailbox, mbox.Messages)

	seqSet := new(imap.SeqSet)
	seqSet.AddRange(1, mbox.Messages)

	section := imap.BodySectionName{}
	messages := make(chan *imap.Message, mbox.Messages)
	done := make(chan error, 1)
	go func() {
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
