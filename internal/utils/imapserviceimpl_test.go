package utils

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/emersion/go-imap"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetMailboxesX(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMockClient(ctrl)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()

	service, err := NewImapService(
		WithAuth("foo", "bar"),
		WithClient(mockClient),
		WithLogger(logger),
		WithCtx(ctx),
	)

	if err != nil {
		t.Fatal(err)
	}

	// Setting up the expected calls and returns
	// mailboxChan := make(chan *imap.MailboxInfo, 10)
	doneChan := make(chan error, 1)

	mockClient.EXPECT().
		Login("foo", "bar")

	mockClient.EXPECT().
		Logout()

	mockClient.EXPECT().
		List("", "*", gomock.Any()).
		Do(func(_, _ string, ch interface{}) {
			mCh, ok := ch.(chan *imap.MailboxInfo)
			if !ok {
				t.Fatalf("Type assertion failed: Expected chan *imap.MailboxInfo, got %T", ch)
			}
			go func() {
				mCh <- &imap.MailboxInfo{Name: "Folder1"}
				mCh <- &imap.MailboxInfo{Name: "Folder2"}
				mCh <- &imap.MailboxInfo{Name: "Folder3"}
				mCh <- &imap.MailboxInfo{Name: "Folder4"}
				mCh <- &imap.MailboxInfo{Name: "Folder5"}
				close(mCh)
			}()
			doneChan <- nil
			close(doneChan)
		}).
		Return(nil)

	// Call the function to test
	result, err := service.GetMailboxes()

	if err != nil {
		t.Fatal(err)
	}

	// Define expected results
	expected := map[string]Mailbox{
		"Folder1": {Name: "Folder1", Delete: false, Export: false},
		"Folder2": {Name: "Folder2", Delete: false, Export: false},
		"Folder3": {Name: "Folder3", Delete: false, Export: false},
		"Folder4": {Name: "Folder4", Delete: false, Export: false},
		"Folder5": {Name: "Folder5", Delete: false, Export: false},
	}

	// Check if the results meet the expectations
	assert.Equal(t, expected, result, "The returned map of mailboxes should match the expected values.")
}
