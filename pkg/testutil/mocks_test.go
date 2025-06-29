package testutil

import (
	"testing"

	"aaronromeo.com/postmanpat/pkg/base"
	"aaronromeo.com/postmanpat/pkg/models/mailbox"
	"github.com/stretchr/testify/assert"
)

func TestMockImapManager(t *testing.T) {
	mock := &MockImapManager{
		GetMailboxesFunc: func() (map[string]*mailbox.MailboxImpl, error) {
			return map[string]*mailbox.MailboxImpl{
				"test": {
					SerializedMailbox: base.SerializedMailbox{
						Name: "test",
					},
				},
			}, nil
		},
	}

	result, err := mock.GetMailboxes()
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Contains(t, result, "test")
}

func TestMockFileManager(t *testing.T) {
	mock := NewMockFileManager()
	
	// Test WriteFile
	err := mock.WriteFile("test.txt", []byte("content"), 0644)
	assert.NoError(t, err)
	assert.Equal(t, []byte("content"), mock.WrittenFiles["test.txt"])
	assert.Equal(t, 0644, int(mock.WrittenPerms["test.txt"]))
	
	// Test ReadFile
	data, err := mock.ReadFile("test.txt")
	assert.NoError(t, err)
	assert.Equal(t, []byte("content"), data)
	assert.Contains(t, mock.ReadFiles, "test.txt")
}

func TestTestMailboxData(t *testing.T) {
	assert.Equal(t, "INBOX", TestMailboxData.INBOX.Name)
	assert.Equal(t, "Sent", TestMailboxData.Sent.Name)
	assert.Equal(t, "Drafts", TestMailboxData.Drafts.Name)
	assert.Equal(t, "Folder/With/Slashes", TestMailboxData.WithSpecialChars.Name)
}

func TestTestSerializedMailboxData(t *testing.T) {
	assert.Contains(t, TestSerializedMailboxData, "INBOX")
	assert.Contains(t, TestSerializedMailboxData, "Sent")
	assert.Contains(t, TestSerializedMailboxData, "Drafts")
	assert.Equal(t, "INBOX", TestSerializedMailboxData["INBOX"].Name)
}
