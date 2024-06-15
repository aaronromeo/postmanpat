package mailbox

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"aaronromeo.com/postmanpat/pkg/utils"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-message"
	"github.com/pkg/errors"
)

const EMAIL_EXPORT_TIMESTAMP_FORMAT = "20060102150405"

type ExportedEmailContainer struct {
	bcc                  []*imap.Address
	cc                   []*imap.Address
	extractedFileName    string
	from                 []*imap.Address
	inReplyTo            string
	mailboxName          string
	messageId            string
	msgBody              []byte
	msgBodyContentType   string
	msgBodyPartPosition  int
	msgBodyPartSpecifier string
	replyTo              []*imap.Address
	sender               []*imap.Address
	subject              string
	timestamp            time.Time
	to                   []*imap.Address
}

type ExportedEmailMetadata struct {
	Subject     string    `json:"subject"`
	From        string    `json:"from"`
	To          string    `json:"to"`
	CC          string    `json:"cc"`
	BCC         string    `json:"bcc"`
	Timestamp   time.Time `json:"timestamp"`
	MessageId   string    `json:"messageId"`
	InReplyTo   string    `json:"inReplyTo"`
	MailboxName string    `json:"mailboxName"`
}

func (e ExportedEmailContainer) WriteToFile(mlogger *slog.Logger, fileManager utils.FileManager, baseFolder string) error {
	basePath := filepath.Join(baseFolder, sanitize(e.mailboxName))

	// Unique folder for each email
	emailFolderName := fmt.Sprintf("%s-%s-%x", e.timestamp.Format("20060102T150405Z"), sanitize(e.subject), md5.Sum([]byte(e.messageId)))
	emailFolderPath := filepath.Join(basePath, emailFolderName)
	err := fileManager.MkdirAll(emailFolderPath, os.ModePerm)
	if err != nil {
		mlogger.Error("Failed to create email folder", slog.Any("error", err))
		return err
	}

	metadataFile := filepath.Join(emailFolderPath, "metadata.json")
	var from []string
	for _, f := range e.from {
		from = append(from, f.Address())
	}
	var to []string
	for _, f := range e.to {
		to = append(to, f.Address())
	}
	var cc []string
	for _, f := range e.cc {
		cc = append(cc, f.Address())
	}
	var bcc []string
	for _, f := range e.bcc {
		bcc = append(bcc, f.Address())
	}
	metadata := ExportedEmailMetadata{
		Subject:     e.subject,
		From:        strings.Join(from, ", "),
		To:          strings.Join(to, ", "),
		CC:          strings.Join(cc, ", "),
		BCC:         strings.Join(bcc, ", "),
		Timestamp:   e.timestamp,
		MessageId:   e.messageId,
		InReplyTo:   e.inReplyTo,
		MailboxName: e.mailboxName,
	}
	metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		mlogger.Error("Failed to serialize metadata", slog.Any("error", err))
		return err
	}
	err = fileManager.WriteFile(metadataFile, metadataBytes, os.ModePerm)
	if err != nil {
		mlogger.Error("Failed to write metadata file", slog.Any("error", err))
		return err
	}

	// Save email body
	bodyFile := filepath.Join(emailFolderPath, fmt.Sprintf("body.%s", getExtension(e.msgBodyContentType)))
	writer, err := fileManager.Create(bodyFile)
	if err != nil {
		mlogger.Error("Failed to create body file", slog.Any("error", err))
		return err
	}

	// mlogger.Info(e.mailboxName, "messageBody", string(e.msgBody[:]))
	_, err = writer.Write(e.msgBody)
	if err != nil {
		mlogger.Error(
			err.Error(),
			slog.Any("error", utils.WrapError(err)),
			slog.Any("fileName", bodyFile),
			slog.Any("buffer", e.msgBody),
		)
		return err
	}

	if err = writer.Flush(); err != nil {
		mlogger.Error(err.Error(), slog.Any("error", utils.WrapError(err)))
	}

	if err != nil {
		mlogger.Error("Failed to write body file", slog.Any("error", err))
		return err
	}

	// Save attachments (if any)
	if len(e.extractedFileName) > 0 {
		attachmentFile := filepath.Join(emailFolderPath, sanitize(e.extractedFileName))
		err = fileManager.WriteFile(attachmentFile, e.msgBody, os.ModePerm)
		if err != nil {
			mlogger.Error("Failed to write attachment file", slog.Any("error", err))
			return err
		}
	}

	return nil
}

func getExtension(contentType string) string {
	switch contentType {
	case "text/html":
		return "html"

	case "text/plain":
		return "txt"

	case "application/msword":
		return "doc"

	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx"

	case "application/zip":
		return "zip"

	case "multipart/alternative":
		return "wtf"

	case "application/octet-stream":
		return "eml"

	default:
		return "eml"
	}
}

func sanitize(input string) string {
	illegalCharsRe := regexp.MustCompile(`[^a-zA-Z0-9\-_]`)
	return illegalCharsRe.ReplaceAllString(input, "_")
}

func ExportedEmailContainerFactory(mailboxName string, msg *imap.Message) ([]ExportedEmailContainer, error) {
	containers := []ExportedEmailContainer{}
	for bodySectionName, literal := range msg.Body {
		convertedContainers, err := convertBodySectionToContainers(mailboxName, msg.Envelope.Date, bodySectionName.BodyPartName.Specifier, literal.(io.Reader))
		if err != nil {
			return nil, err
		}
		containers = append(containers, convertedContainers...)
	}

	newContainers := make([]ExportedEmailContainer, len(containers))
	for i, _ := range containers {
		newContainers[i].bcc = msg.Envelope.Bcc
		newContainers[i].cc = msg.Envelope.Cc
		newContainers[i].from = msg.Envelope.From
		newContainers[i].inReplyTo = msg.Envelope.InReplyTo
		newContainers[i].messageId = msg.Envelope.MessageId
		newContainers[i].replyTo = msg.Envelope.ReplyTo
		newContainers[i].sender = msg.Envelope.Sender
		newContainers[i].subject = msg.Envelope.Subject
		newContainers[i].to = msg.Envelope.To
		newContainers[i].timestamp = msg.Envelope.Date
		newContainers[i].mailboxName = mailboxName
	}

	return newContainers, nil
}

func convertBodySectionToContainers(mailboxName string, tstamp time.Time, partSpecifier imap.PartSpecifier, messageReader io.Reader) ([]ExportedEmailContainer, error) {
	messageEntity, err := message.Read(messageReader)
	if message.IsUnknownCharset(err) {
		// This error is not fatal
		return nil, errors.Errorf("Unknown encoding: %v", err)
	} else if err != nil {
		return nil, err
	}

	containers := []ExportedEmailContainer{}

	if messageMultiPartReader := messageEntity.MultipartReader(); messageMultiPartReader != nil {
		// This is a multipart message
		partCount := 1
		for {
			p, err := messageMultiPartReader.NextPart()
			switch {
			case errors.Is(err, io.EOF):
				return containers, nil
			case err != nil && strings.Contains(err.Error(), "multipart: NextPart: EOF"):
				return containers, nil
			case err == nil:
				messageEntity := *p
				header := messageEntity.Header
				contentType, params, err := header.ContentType()
				if err != nil {
					return nil, err
				}
				messageBody, err := io.ReadAll((*p).Body)

				if err != nil {
					return nil, err
				}

				if len(messageBody) == 0 { // Skip empty parts
					continue
				}

				containers = append(containers, ExportedEmailContainer{
					timestamp:            tstamp,
					msgBodyPartSpecifier: string(partSpecifier),
					msgBodyPartPosition:  partCount,
					msgBodyContentType:   contentType,
					mailboxName:          mailboxName,
					extractedFileName:    params["name"],
					msgBody:              messageBody,
				})
			default:
				return nil, err
			}
			partCount++
		}
	} else {
		header := messageEntity.Header
		contentType, params, err := header.ContentType()
		if err != nil {
			return nil, err
		}
		messageBody, err := io.ReadAll(messageEntity.Body)

		if err != nil {
			return nil, err
		}

		if len(messageBody) == 0 { // Skip empty parts
			return nil, nil
		}

		containers = append(containers, ExportedEmailContainer{
			timestamp:            tstamp,
			msgBodyPartSpecifier: string(partSpecifier),
			msgBodyPartPosition:  1,
			msgBodyContentType:   contentType,
			mailboxName:          mailboxName,
			extractedFileName:    params["name"],
			msgBody:              messageBody,
		})

	}

	return containers, nil
}

func removeEmptyStrings(s []string) []string {
	var r []string
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}
