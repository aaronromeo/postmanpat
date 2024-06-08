package mailbox

import (
	"context"
	"fmt"
	"testing"
	"time"

	"aaronromeo.com/postmanpat/pkg/base"
	"aaronromeo.com/postmanpat/pkg/mock"
	"github.com/emersion/go-imap"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestNewMailbox(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockClient(ctrl)
	logger := mock.SetupLogger(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		options []MailboxOption
		wantErr bool
	}{
		{
			name: "valid configuration",
			options: []MailboxOption{
				WithClient(mockClient),
				WithLogger(logger),
				WithCtx(ctx),
				WithLoginFn(func() (base.Client, error) { return mockClient, nil }),
				WithLogoutFn(func() error { return nil }),
				WithFileManager(mock.MockFileWriter{}),
			},
			wantErr: false,
		},
		{
			name: "missing client",
			options: []MailboxOption{
				WithLogger(logger),
				WithCtx(ctx),
				WithLoginFn(func() (base.Client, error) { return mockClient, nil }),
				WithLogoutFn(func() error { return nil }),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMailbox(tt.options...)
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
	mockfileManager := mock.MockFileWriter{Writers: &[]mock.MockWriter{}}

	// Setup the Mailbox with mock dependencies
	mb := &MailboxImpl{
		Name:        "INBOX",
		client:      mockClient,
		logger:      logger,
		ctx:         ctx,
		loginFn:     func() (base.Client, error) { return mockClient, nil },
		logoutFn:    func() error { return nil },
		fileManager: mockfileManager,
	}

	// Mock mailbox status with 10 messages
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

	// Prepare the mocked message fetching
	mockClient.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(seqset *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
			defer close(ch)
			for i := 0; i < int(mboxStatus.Messages); i++ {
				msg := &imap.Message{
					Envelope: &imap.Envelope{Subject: "Test Subject", Date: time.Now()},
					Body: map[*imap.BodySectionName]imap.Literal{
						{}: mock.NewStringLiteral("Subject: Your Name\r\n" +
							"Content-Type: multipart/mixed; boundary=message-boundary\r\n" +
							"\r\n" +
							"--message-boundary\r\n" +
							"Content-Type: multipart/alternative; boundary=text-boundary\r\n" +
							"\r\n" +
							"--text-boundary\r\n" +
							"Content-Type: text/plain\r\n" +
							"Content-Disposition: inline\r\n" +
							"\r\n" +
							"Who are you?\r\n" +
							"--text-boundary--\r\n" +
							"--message-boundary\r\n" +
							"Content-Type: text/plain\r\n" +
							"Content-Disposition: attachment; filename=note.txt\r\n" +
							"\r\n" +
							fmt.Sprintf("I'm %s.\r\n", names[i]) +
							"--message-boundary--\r\n"),
					},
				}
				ch <- msg
			}
			return nil
		},
	)

	// Verify all expected interactions
	// mockClient.EXPECT().Logout().Return(nil).Times(1)

	// Test ExportMessages
	err := mb.ExportMessages()
	assert.NoError(t, err)

	assert.Equal(t, 10, len(*mockfileManager.Writers))
	for i, name := range names {
		assert.Contains(t, (*mockfileManager.Writers)[i].Buffer.String(), fmt.Sprintf("I'm %s.", name))
	}
}
