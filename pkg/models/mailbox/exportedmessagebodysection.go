package mailbox

import (
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

func CreateExportedEmailMetadata(msg *imap.Message, mailboxName string) ExportedEmailMetadata {
	var from []string
	for _, f := range msg.Envelope.From {
		from = append(from, f.Address())
	}
	var to []string
	for _, f := range msg.Envelope.To {
		to = append(to, f.Address())
	}
	var cc []string
	for _, f := range msg.Envelope.Cc {
		cc = append(cc, f.Address())
	}
	var bcc []string
	for _, f := range msg.Envelope.Bcc {
		bcc = append(bcc, f.Address())
	}

	return ExportedEmailMetadata{
		Subject:     msg.Envelope.Subject,
		From:        strings.Join(removeEmptyStrings(from), ", "),
		To:          strings.Join(removeEmptyStrings(to), ", "),
		CC:          strings.Join(removeEmptyStrings(cc), ", "),
		BCC:         strings.Join(removeEmptyStrings(bcc), ", "),
		Timestamp:   msg.InternalDate,
		MessageId:   msg.Envelope.MessageId,
		InReplyTo:   msg.Envelope.InReplyTo,
		MailboxName: mailboxName,
	}
}

type ExportedEmailContainer struct {
	extractedFileName    string
	mailboxName          string
	msgBody              []byte
	msgBodyContentType   string
	msgBodyPartPosition  int
	msgBodyPartSpecifier string
}

func (e ExportedEmailContainer) WriteToFile(mlogger *slog.Logger, fileManager utils.FileManager, emailFolderPath string) error {
	// Save email body
	bodyFilename := filepath.Join(emailFolderPath, fmt.Sprintf("body_%d.%s", e.msgBodyPartPosition, getExtension(e.msgBodyContentType)))
	writer, err := fileManager.Create(bodyFilename)
	if err != nil {
		mlogger.Error("Failed to create body file", slog.Any("error", err))
		return err
	}

	_, err = writer.Write(e.msgBody)
	if err != nil {
		mlogger.Error(
			err.Error(),
			slog.Any("error", utils.WrapError(err)),
			slog.Any("fileName", bodyFilename),
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
		r, ok := literal.(io.Reader)
		if !ok {
			return nil, errors.New("unable to convert literal to io.Reader")
		}
		convertedContainers, err := convertBodySectionToContainers(mailboxName, bodySectionName.BodyPartName.Specifier, r)
		if err != nil {
			return nil, err
		}
		containers = append(containers, convertedContainers...)
	}

	newContainers := make([]ExportedEmailContainer, len(containers))
	for i := range containers {
		newContainers[i].mailboxName = mailboxName
		newContainers[i].msgBody = containers[i].msgBody
		newContainers[i].msgBodyContentType = containers[i].msgBodyContentType
		newContainers[i].msgBodyPartPosition = containers[i].msgBodyPartPosition
		newContainers[i].msgBodyPartSpecifier = containers[i].msgBodyPartSpecifier
		newContainers[i].extractedFileName = containers[i].extractedFileName
	}

	return newContainers, nil
}

func convertBodySectionToContainers(mailboxName string, partSpecifier imap.PartSpecifier, messageReader io.Reader) ([]ExportedEmailContainer, error) {
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
