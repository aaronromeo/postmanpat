package mailbox_test

import (
	"context"
	"strings"
	"testing"
	"time"

	// "aaronromeo.com/postmanpat/pkg/mailbox"
	"github.com/emersion/go-imap"
	"go.uber.org/mock/gomock"

	"aaronromeo.com/postmanpat/pkg/base"
	"aaronromeo.com/postmanpat/pkg/mock"
	"aaronromeo.com/postmanpat/pkg/models/mailbox"
	// "github.com/emersion/go-imap"
	// "github.com/stretchr/testify/assert"
	// "go.uber.org/mock/gomock"
)

func TestNewMailbox(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockClient(ctrl)
	logger := mock.SetupLogger(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		options []mailbox.MailboxOption
		wantErr bool
	}{
		{
			name: "valid configuration",
			options: []mailbox.MailboxOption{
				mailbox.WithClient(mockClient),
				mailbox.WithLogger(logger),
				mailbox.WithCtx(ctx),
				mailbox.WithLoginFn(func() (base.Client, error) { return mockClient, nil }),
				mailbox.WithLogoutFn(func() error { return nil }),
				mailbox.WithFileManager(mock.MockFileWriter{}),
			},
			wantErr: false,
		},
		{
			name: "missing client",
			options: []mailbox.MailboxOption{
				mailbox.WithLogger(logger),
				mailbox.WithCtx(ctx),
				mailbox.WithLoginFn(func() (base.Client, error) { return mockClient, nil }),
				mailbox.WithLogoutFn(func() error { return nil }),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mailbox.NewMailbox(tt.options...)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMailbox() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExportMessages(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	type test struct {
		name             string
		messages         []*imap.Message
		wantFileContents map[string]string
	}

	tests := []test{
		{
			name: "Multiple multi-part emails in inbox",
			messages: []*imap.Message{
				{
					SeqNum:       4,
					InternalDate: time.Date(2021, 3, 15, 12, 34, 56, 0, time.UTC),
					Envelope: &imap.Envelope{
						Subject: "Test Subject Beethoven",
						From: []*imap.Address{
							{PersonalName: "Ludwig van Beethoven", MailboxName: "beethoven", HostName: "beethoven.com"},
						},
						To: []*imap.Address{
							{PersonalName: "Recipient", MailboxName: "recipient", HostName: "example.com"},
						},
						Date:      time.Date(2021, 3, 15, 12, 34, 56, 0, time.UTC),
						MessageId: "A5166670-8640-4F87-B002-9C2AD331004F",
					},
					Body: map[*imap.BodySectionName]imap.Literal{
						{}: mock.NewStringLiteral(
							"Subject: Test Subject Beethoven\r\n" +
								"Content-Type: multipart/mixed; boundary=message-boundary\r\n" +
								"\r\n" +
								"--message-boundary\r\n" +
								"Content-Type: text/plain\r\n" +
								"Content-Disposition: inline\r\n" +
								"\r\n" +
								"Who are you? I'm Ludwig van Beethoven.\r\n" +
								"--message-boundary\r\n" +
								"Content-Type: application/octet-stream\r\n" +
								"Content-Disposition: attachment; filename=note.txt\r\n" +
								"\r\n" +
								"Attachment content of something Beethoven related.\r\n" +
								"--message-boundary--\r\n",
						),
					},
				},
				{
					SeqNum:       5,
					InternalDate: time.Date(2023, 8, 22, 14, 18, 30, 0, time.UTC),
					Envelope: &imap.Envelope{
						Subject: "Test Subject Chopin",
						From: []*imap.Address{
							{PersonalName: "Frédéric Chopin", MailboxName: "chopin", HostName: "chopin.com"},
						},
						To: []*imap.Address{
							{PersonalName: "Recipient", MailboxName: "recipient", HostName: "example.com"},
						},
						Date:      time.Date(2023, 8, 22, 14, 18, 30, 0, time.UTC),
						MessageId: "5AA34A5D-7E77-43FF-A95F-3DE3A1CB4AC4",
					},
					Body: map[*imap.BodySectionName]imap.Literal{
						{}: mock.NewStringLiteral(
							"Subject: Test Subject Chopin\r\n" +
								"Content-Type: multipart/mixed; boundary=message-boundary\r\n" +
								"\r\n" +
								"--message-boundary\r\n" +
								"Content-Type: text/plain\r\n" +
								"Content-Disposition: inline\r\n" +
								"\r\n" +
								"Who are you? I'm Frédéric Chopin.\r\n" +
								"--message-boundary\r\n" +
								"Content-Type: application/octet-stream\r\n" +
								"Content-Disposition: attachment; filename=note.txt\r\n" +
								"\r\n" +
								"Attachment content of something Chopin related.\r\n" +
								"--message-boundary--\r\n",
						),
					},
				},
			},
			wantFileContents: map[string]string{
				"exportedemails/INBOX/20210315T123456Z-Test_Subject_Beethoven-60e4e2abcbc4c6971acbae5201788dd8/metadata.json": `{
  "subject": "Test Subject Beethoven",
  "from": "beethoven@beethoven.com",
  "to": "recipient@example.com",
  "cc": "",
  "bcc": "",
  "timestamp": "2021-03-15T12:34:56Z",
  "messageId": "A5166670-8640-4F87-B002-9C2AD331004F",
  "inReplyTo": "",
  "mailboxName": "INBOX"
}`,
				"exportedemails/INBOX/20210315T123456Z-Test_Subject_Beethoven-60e4e2abcbc4c6971acbae5201788dd8/body_1.txt": `Who are you? I'm Ludwig van Beethoven.`,
				"exportedemails/INBOX/20210315T123456Z-Test_Subject_Beethoven-60e4e2abcbc4c6971acbae5201788dd8/body_2.eml": `Attachment content of something Beethoven related.`,
				"exportedemails/INBOX/20230822T141830Z-Test_Subject_Chopin-884614d733b3fda61802860e5b7b25fc/metadata.json": `{
  "subject": "Test Subject Chopin",
  "from": "chopin@chopin.com",
  "to": "recipient@example.com",
  "cc": "",
  "bcc": "",
  "timestamp": "2023-08-22T14:18:30Z",
  "messageId": "5AA34A5D-7E77-43FF-A95F-3DE3A1CB4AC4",
  "inReplyTo": "",
  "mailboxName": "INBOX"
}`,
				"exportedemails/INBOX/20230822T141830Z-Test_Subject_Chopin-884614d733b3fda61802860e5b7b25fc/body_1.txt": `Who are you? I'm Frédéric Chopin.`,
				"exportedemails/INBOX/20230822T141830Z-Test_Subject_Chopin-884614d733b3fda61802860e5b7b25fc/body_2.eml": `Attachment content of something Chopin related.`,
			},
		}, {
			name: "Single email, single plain text part in inbox",
			messages: []*imap.Message{
				{
					SeqNum:       1,
					InternalDate: time.Date(2022, 5, 10, 6, 12, 45, 0, time.UTC),
					Envelope: &imap.Envelope{
						Subject: "Plain Text Email",
						From: []*imap.Address{
							{PersonalName: "Ludwig van Beethoven", MailboxName: "beethoven", HostName: "beethoven.com"},
						},
						To: []*imap.Address{
							{PersonalName: "Recipient", MailboxName: "recipient", HostName: "example.com"},
						},
						Date:      time.Date(2021, 3, 15, 12, 34, 56, 0, time.UTC),
						MessageId: "28F7274B-F6B1-45EA-AD31-69EDCB5DE32C",
					},
					Body: map[*imap.BodySectionName]imap.Literal{
						{}: mock.NewStringLiteral("Subject: Plain Text Email\r\n\r\nHello, this is a plain text email.\r\n"),
					},
				},
			},
			wantFileContents: map[string]string{
				"exportedemails/INBOX/20220510T061245Z-Plain_Text_Email-4bd44a2a01f19b1e6600c1a4d9e0ab3d/metadata.json": `{
  "subject": "Plain Text Email",
  "from": "beethoven@beethoven.com",
  "to": "recipient@example.com",
  "cc": "",
  "bcc": "",
  "timestamp": "2022-05-10T06:12:45Z",
  "messageId": "28F7274B-F6B1-45EA-AD31-69EDCB5DE32C",
  "inReplyTo": "",
  "mailboxName": "INBOX"
}`,
				"exportedemails/INBOX/20220510T061245Z-Plain_Text_Email-4bd44a2a01f19b1e6600c1a4d9e0ab3d/body_1.txt": `Hello, this is a plain text email.
`,
			},
		}, {
			name: "Single email, single part HTML in inbox",
			messages: []*imap.Message{
				{
					SeqNum:       1,
					InternalDate: time.Date(2022, 5, 10, 6, 12, 45, 0, time.UTC),
					Envelope: &imap.Envelope{
						Subject: "HTML Email",
						From: []*imap.Address{
							{PersonalName: "Ludwig van Beethoven", MailboxName: "beethoven", HostName: "beethoven.com"},
						},
						To: []*imap.Address{
							{PersonalName: "Recipient", MailboxName: "recipient", HostName: "example.com"},
						},
						Date:      time.Date(2021, 3, 15, 12, 34, 56, 0, time.UTC),
						MessageId: "28F7274B-F6B1-45EA-AD31-69EDCB5DE32C",
					},
					Body: map[*imap.BodySectionName]imap.Literal{
						{}: mock.NewStringLiteral("Subject: HTML Email\r\nContent-Type: text/html\r\n\r\n<p>Hello, this is an HTML email.</p>\r\n"),
					},
				},
			},
			wantFileContents: map[string]string{
				"exportedemails/INBOX/20220510T061245Z-HTML_Email-9eecf1f98b33e0eb9a85a3de45223a8d/metadata.json": `{
  "subject": "HTML Email",
  "from": "beethoven@beethoven.com",
  "to": "recipient@example.com",
  "cc": "",
  "bcc": "",
  "timestamp": "2022-05-10T06:12:45Z",
  "messageId": "28F7274B-F6B1-45EA-AD31-69EDCB5DE32C",
  "inReplyTo": "",
  "mailboxName": "INBOX"
}`,
				"exportedemails/INBOX/20220510T061245Z-HTML_Email-9eecf1f98b33e0eb9a85a3de45223a8d/body_1.html": `<p>Hello, this is an HTML email.</p>`,
			},
		}, {
			name: "Single email, single part Mixed Content in inbox",
			messages: []*imap.Message{
				{
					SeqNum:       1,
					InternalDate: time.Date(2022, 5, 10, 6, 12, 45, 0, time.UTC),
					Envelope: &imap.Envelope{
						Subject: "Mixed Content",
						From: []*imap.Address{
							{PersonalName: "Ludwig van Beethoven", MailboxName: "beethoven", HostName: "beethoven.com"},
						},
						To: []*imap.Address{
							{PersonalName: "Recipient", MailboxName: "recipient", HostName: "example.com"},
						},
						Date:      time.Date(2024, 5, 20, 4, 30, 15, 0, time.UTC),
						MessageId: "BB5E82CE-DA2C-4CA2-BE24-CA1472428FE0",
					},
					Body: map[*imap.BodySectionName]imap.Literal{
						{}: mock.NewStringLiteral("Subject: Mixed Content\r\nContent-Type: multipart/mixed; boundary=mixed-boundary\r\n\r\n--mixed-boundary\r\nContent-Type: text/plain\r\n\r\nHello, this is text part.\r\n--mixed-boundary\r\nContent-Type: text/html\r\n\r\n<p>Hello, this is HTML part.</p>\r\n--mixed-boundary--\r\n"),
					},
				},
			},
			wantFileContents: map[string]string{
				"exportedemails/INBOX/20220510T061245Z-Mixed_Content-a4873d8180ccba2c487f47eb6a0bb8c3/metadata.json": `{
  "subject": "Mixed Content",
  "from": "beethoven@beethoven.com",
  "to": "recipient@example.com",
  "cc": "",
  "bcc": "",
  "timestamp": "2022-05-10T06:12:45Z",
  "messageId": "BB5E82CE-DA2C-4CA2-BE24-CA1472428FE0",
  "inReplyTo": "",
  "mailboxName": "INBOX"
}`,
				"exportedemails/INBOX/20220510T061245Z-Mixed_Content-a4873d8180ccba2c487f47eb6a0bb8c3/body_1.txt":  "Hello, this is text part.",
				"exportedemails/INBOX/20220510T061245Z-Mixed_Content-a4873d8180ccba2c487f47eb6a0bb8c3/body_2.html": "<p>Hello, this is HTML part.</p>",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := mock.NewMockClient(ctrl)
			logger := mock.SetupLogger(t)
			ctx := context.Background()
			mockfileManager := mock.MockFileWriter{Writers: map[string]mock.MockWriter{}}

			mb := &mailbox.MailboxImpl{
				Name:        "INBOX",
				LoginFn:     func() (base.Client, error) { return mockClient, nil },
				LogoutFn:    func() error { return nil },
				Client:      mockClient,
				Logger:      logger,
				Ctx:         ctx,
				FileManager: mockfileManager,
			}

			mboxStatus := &imap.MailboxStatus{Messages: (uint32)(len(tc.messages))}
			mockClient.EXPECT().Select("INBOX", false).Return(mboxStatus, nil)
			mockClient.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(seqset *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
					defer close(ch)

					for i := 0; i < int(mboxStatus.Messages); i++ {
						ch <- tc.messages[i]
					}
					return nil
				},
			)

			// Export messages and check results
			err := mb.ExportMessages()
			if err != nil {
				t.Fatalf("Unexpected error %+v", err)
			}

			if len(tc.wantFileContents) != len(mockfileManager.Writers) {
				fileNames := []string{}
				for key := range mockfileManager.Writers {
					fileNames = append(fileNames, key)
				}
				t.Fatalf("Incorrect file count. want: %d got: %d\n%+v", len(tc.wantFileContents), len(mockfileManager.Writers), fileNames)
			}

			for key, expectedBody := range tc.wantFileContents {
				actualMockWriter, ok := mockfileManager.Writers[key]
				if !ok {
					t.Fatalf("Missing expected file %s from the exported files", key)
				}

				actualBody := actualMockWriter.Buffer.String()
				if strings.TrimSpace(actualBody) != strings.TrimSpace(expectedBody) {
					t.Fatalf("Exported file %s mismatch. got: `%s` want: `%s` ", key, actualBody, expectedBody)
				}
			}
		})
	}
}
