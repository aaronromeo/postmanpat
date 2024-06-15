package mailbox_test

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	// "aaronromeo.com/postmanpat/pkg/mailbox"
	"github.com/emersion/go-imap"
	"github.com/stretchr/testify/assert"
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

	mboxStatus := &imap.MailboxStatus{Messages: 10}
	mockClient.EXPECT().Select("INBOX", false).Return(mboxStatus, nil)

	names := [10]string{
		"Ludwig van Beethoven",
		"Frédéric Chopin",
		"Franz Liszt",
		"Clara Schumann",
		"Sergei Rachmaninoff",
		"Arthur Rubinstein",
		"Vladimir Horowitz",
		"Glenn Gould",
		"Martha Argerich",
		"Lang Lang",
	}

	mockClient.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(seqset *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
			defer close(ch)
			start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)

			for i := 0; i < int(mboxStatus.Messages); i++ {
				msg := &imap.Message{
					SeqNum:       uint32(i + 1),
					InternalDate: randomTime(start, end),
					Envelope: &imap.Envelope{
						Subject: fmt.Sprintf("Test Subject %d", i),
						From: []*imap.Address{
							{PersonalName: names[i], MailboxName: "example", HostName: "example.com"},
						},
						To: []*imap.Address{
							{PersonalName: "Recipient", MailboxName: "recipient", HostName: "example.com"},
						},
						Date: time.Now(),
					},
					Body: map[*imap.BodySectionName]imap.Literal{
						{}: mock.NewStringLiteral(fmt.Sprintf(
							"Subject: Test Subject %d\r\n"+
								"Content-Type: multipart/mixed; boundary=message-boundary\r\n"+
								"\r\n"+
								"--message-boundary\r\n"+
								"Content-Type: text/plain\r\n"+
								"Content-Disposition: inline\r\n"+
								"\r\n"+
								"Who are you? I'm %s.\r\n"+
								"--message-boundary\r\n"+
								"Content-Type: application/octet-stream\r\n"+
								"Content-Disposition: attachment; filename=note.txt\r\n"+
								"\r\n"+
								"Attachment content %d.\r\n"+
								"--message-boundary--\r\n",
							i, names[i], i)),
					},
				}
				ch <- msg
			}
			return nil
		},
	)

	// Export messages and check results
	err := mb.ExportMessages()
	assert.NoError(t, err)

	assert.Equal(t, 10, len(mockfileManager.Writers))

	// Validate the contents of exported messages
	for _, writer := range mockfileManager.Writers {
		content := writer.Buffer.String()
		assert.Contains(t, content, "Test Subject")
		assert.Contains(t, content, "Who are you?")
	}

	// Additional robust tests

	// Test different content types
	mockClient.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(seqset *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
			defer close(ch)

			// Plain text email
			ch <- &imap.Message{
				SeqNum:   1,
				Envelope: &imap.Envelope{Subject: "Plain Text Email"},
				Body: map[*imap.BodySectionName]imap.Literal{
					{}: mock.NewStringLiteral("Subject: Plain Text\r\n\r\nHello, this is a plain text email.\r\n"),
				},
			}

			// HTML email
			ch <- &imap.Message{
				SeqNum:   2,
				Envelope: &imap.Envelope{Subject: "HTML Email"},
				Body: map[*imap.BodySectionName]imap.Literal{
					{}: mock.NewStringLiteral("Subject: HTML Email\r\nContent-Type: text/html\r\n\r\n<p>Hello, this is an HTML email.</p>\r\n"),
				},
			}

			// Mixed content email
			ch <- &imap.Message{
				SeqNum:   3,
				Envelope: &imap.Envelope{Subject: "Mixed Content Email"},
				Body: map[*imap.BodySectionName]imap.Literal{
					{}: mock.NewStringLiteral("Subject: Mixed Content\r\nContent-Type: multipart/mixed; boundary=mixed-boundary\r\n\r\n--mixed-boundary\r\nContent-Type: text/plain\r\n\r\nHello, this is text part.\r\n--mixed-boundary\r\nContent-Type: text/html\r\n\r\n<p>Hello, this is HTML part.</p>\r\n--mixed-boundary--\r\n"),
				},
			}

			return nil
		},
	)

	err = mb.ExportMessages()
	assert.NoError(t, err)

	// Verify that different content types were processed correctly
	assert.Contains(t, mockfileManager.Writers["Plain Text Email"].Buffer.String(), "Hello, this is a plain text email.")
	assert.Contains(t, mockfileManager.Writers["HTML Email"].Buffer.String(), "<p>Hello, this is an HTML email.</p>")
	assert.Contains(t, mockfileManager.Writers["Mixed Content Email"].Buffer.String(), "Hello, this is text part.")
	assert.Contains(t, mockfileManager.Writers["Mixed Content Email"].Buffer.String(), "<p>Hello, this is HTML part.</p>")

	// Test error handling for file creation
	// mockfileManager = mock.MockFileWriter{
	// 	Create: func(name string) (utils.FileWriter, error) {
	// 		if filepath.Base(name) == "body.txt" {
	// 			return nil, fmt.Errorf("simulated file creation error")
	// 		}
	// 		return mock.MockWriter{}, nil
	// 	},
	// }

	mb.FileManager = mockfileManager

	mockClient.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(seqset *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
			defer close(ch)

			// Email with text body
			ch <- &imap.Message{
				SeqNum:   1,
				Envelope: &imap.Envelope{Subject: "Error Test Email"},
				Body: map[*imap.BodySectionName]imap.Literal{
					{}: mock.NewStringLiteral("Subject: Error Test\r\n\r\nThis will fail.\r\n"),
				},
			}

			return nil
		},
	)

	err = mb.ExportMessages()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "simulated file creation error")
}

func randomTime(start, end time.Time) time.Time {
	delta := end.Sub(start)
	sec := rand.Int63n(int64(delta.Seconds()))
	return start.Add(time.Duration(sec) * time.Second)
}
