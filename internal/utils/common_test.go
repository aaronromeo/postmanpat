package utils

import (
	"testing"

	"github.com/emersion/go-imap"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetMailboxes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMockClient(ctrl)

	// Setting up the expected calls and returns
	// mailboxChan := make(chan *imap.MailboxInfo, 10)
	doneChan := make(chan error, 1)

	mockClient.EXPECT().
		List("", "*", gomock.Any()).
		Do(func(_, _ string, ch interface{}) {
			mCh := ch.(chan *imap.MailboxInfo)
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
	result := GetMailboxes(mockClient)

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
