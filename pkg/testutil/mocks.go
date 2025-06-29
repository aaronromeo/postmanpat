// Package testutil provides testing utilities, mocks, and interfaces for PostmanPat.
// This package follows Go best practices by providing a centralized location for
// test-related types and utilities that can be shared across test files.
package testutil

import (
	"context"
	"os"

	"aaronromeo.com/postmanpat/pkg/base"
	"aaronromeo.com/postmanpat/pkg/models/mailbox"
	"aaronromeo.com/postmanpat/pkg/utils"
)

// TestableImapManager defines the interface for IMAP manager operations used in testing.
// This interface matches the actual implementation signatures rather than the mismatched
// ImapManager interface in imapmanager.go.
type TestableImapManager interface {
	GetMailboxes() (map[string]*mailbox.MailboxImpl, error)
	UnserializeMailboxes() (map[string]*mailbox.MailboxImpl, error)
}

// MockImapManager provides a mock implementation of TestableImapManager for testing.
// It allows injection of custom behavior through function fields.
type MockImapManager struct {
	GetMailboxesFunc        func() (map[string]*mailbox.MailboxImpl, error)
	UnserializeMailboxesFunc func() (map[string]*mailbox.MailboxImpl, error)
}

// GetMailboxes implements TestableImapManager.GetMailboxes for testing.
func (m *MockImapManager) GetMailboxes() (map[string]*mailbox.MailboxImpl, error) {
	if m.GetMailboxesFunc != nil {
		return m.GetMailboxesFunc()
	}
	return make(map[string]*mailbox.MailboxImpl), nil
}

// UnserializeMailboxes implements TestableImapManager.UnserializeMailboxes for testing.
func (m *MockImapManager) UnserializeMailboxes() (map[string]*mailbox.MailboxImpl, error) {
	if m.UnserializeMailboxesFunc != nil {
		return m.UnserializeMailboxesFunc()
	}
	return make(map[string]*mailbox.MailboxImpl), nil
}

// MockMailbox provides a mock implementation of the Mailbox interface for testing.
type MockMailbox struct {
	ReapFunc                   func() error
	ExportAndDeleteMessagesFunc func() error
	DeleteMessagesFunc         func() error
	SerializeFunc              func() (base.SerializedMailbox, error)
	ProcessMailboxFunc         func(ctx context.Context) error
	
	// Track method calls for verification
	ReapCalled                   bool
	ExportAndDeleteMessagesCalled bool
	DeleteMessagesCalled         bool
	SerializeCalled              bool
	ProcessMailboxCalled         bool
}

// NewMockMailbox creates a new MockMailbox with default implementations.
func NewMockMailbox() *MockMailbox {
	return &MockMailbox{}
}

// Reap implements mailbox.Mailbox.Reap for testing.
func (m *MockMailbox) Reap() error {
	m.ReapCalled = true
	if m.ReapFunc != nil {
		return m.ReapFunc()
	}
	return nil
}

// ExportAndDeleteMessages implements mailbox.Mailbox.ExportAndDeleteMessages for testing.
func (m *MockMailbox) ExportAndDeleteMessages() error {
	m.ExportAndDeleteMessagesCalled = true
	if m.ExportAndDeleteMessagesFunc != nil {
		return m.ExportAndDeleteMessagesFunc()
	}
	return nil
}

// DeleteMessages implements mailbox.Mailbox.DeleteMessages for testing.
func (m *MockMailbox) DeleteMessages() error {
	m.DeleteMessagesCalled = true
	if m.DeleteMessagesFunc != nil {
		return m.DeleteMessagesFunc()
	}
	return nil
}

// Serialize implements mailbox.Mailbox.Serialize for testing.
func (m *MockMailbox) Serialize() (base.SerializedMailbox, error) {
	m.SerializeCalled = true
	if m.SerializeFunc != nil {
		return m.SerializeFunc()
	}
	return base.SerializedMailbox{
		Name:       "MockMailbox",
		Deletable:  false,
		Exportable: false,
		Lifespan:   0,
	}, nil
}

// ProcessMailbox implements mailbox.Mailbox.ProcessMailbox for testing.
func (m *MockMailbox) ProcessMailbox(ctx context.Context) error {
	m.ProcessMailboxCalled = true
	if m.ProcessMailboxFunc != nil {
		return m.ProcessMailboxFunc(ctx)
	}
	return nil
}

// Reset clears all call tracking flags.
func (m *MockMailbox) Reset() {
	m.ReapCalled = false
	m.ExportAndDeleteMessagesCalled = false
	m.DeleteMessagesCalled = false
	m.SerializeCalled = false
	m.ProcessMailboxCalled = false
}

// MockFileManager provides a mock implementation of utils.FileManager for testing.
// It tracks all file operations and allows injection of custom behavior and errors.
type MockFileManager struct {
	WriteFileFunc func(filename string, data []byte, perm os.FileMode) error
	ReadFileFunc  func(filename string) ([]byte, error)
	CloseFunc     func() error
	CreateFunc    func(name string) (utils.Writer, error)
	MkdirAllFunc  func(path string, perm os.FileMode) error
	
	// Track calls for verification in tests
	WrittenFiles map[string][]byte
	WrittenPerms map[string]os.FileMode
	ReadFiles    []string
	CreatedDirs  map[string]os.FileMode
}

// NewMockFileManager creates a new MockFileManager with initialized tracking maps.
func NewMockFileManager() *MockFileManager {
	return &MockFileManager{
		WrittenFiles: make(map[string][]byte),
		WrittenPerms: make(map[string]os.FileMode),
		ReadFiles:    make([]string, 0),
		CreatedDirs:  make(map[string]os.FileMode),
	}
}

// WriteFile implements utils.FileManager.WriteFile for testing.
func (m *MockFileManager) WriteFile(filename string, data []byte, perm os.FileMode) error {
	m.WrittenFiles[filename] = data
	m.WrittenPerms[filename] = perm
	if m.WriteFileFunc != nil {
		return m.WriteFileFunc(filename, data, perm)
	}
	return nil
}

// ReadFile implements utils.FileManager.ReadFile for testing.
func (m *MockFileManager) ReadFile(filename string) ([]byte, error) {
	m.ReadFiles = append(m.ReadFiles, filename)
	if m.ReadFileFunc != nil {
		return m.ReadFileFunc(filename)
	}
	// Return data if it was previously written
	if data, exists := m.WrittenFiles[filename]; exists {
		return data, nil
	}
	return nil, nil
}

// Close implements utils.FileManager.Close for testing.
func (m *MockFileManager) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// Create implements utils.FileManager.Create for testing.
func (m *MockFileManager) Create(name string) (utils.Writer, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(name)
	}
	return nil, nil
}

// MkdirAll implements utils.FileManager.MkdirAll for testing.
func (m *MockFileManager) MkdirAll(path string, perm os.FileMode) error {
	m.CreatedDirs[path] = perm
	if m.MkdirAllFunc != nil {
		return m.MkdirAllFunc(path, perm)
	}
	return nil
}

// Reset clears all tracked operations, useful for test cleanup.
func (m *MockFileManager) Reset() {
	m.WrittenFiles = make(map[string][]byte)
	m.WrittenPerms = make(map[string]os.FileMode)
	m.ReadFiles = make([]string, 0)
	m.CreatedDirs = make(map[string]os.FileMode)
}

// TestableListMailboxNamesFunc defines the signature for a testable version of listMailboxNames.
// This allows dependency injection for testing without modifying the original function.
type TestableListMailboxNamesFunc func(isi TestableImapManager, fileMgr utils.FileManager) error

// Common test data structures for reuse across test files

// TestMailboxData provides common test mailbox configurations.
var TestMailboxData = struct {
	INBOX    *mailbox.MailboxImpl
	Sent     *mailbox.MailboxImpl
	Drafts   *mailbox.MailboxImpl
	WithSpecialChars *mailbox.MailboxImpl
}{
	INBOX: &mailbox.MailboxImpl{
		SerializedMailbox: base.SerializedMailbox{
			Name:       "INBOX",
			Deletable:  false,
			Exportable: true,
			Lifespan:   30,
		},
	},
	Sent: &mailbox.MailboxImpl{
		SerializedMailbox: base.SerializedMailbox{
			Name:       "Sent",
			Deletable:  true,
			Exportable: true,
			Lifespan:   90,
		},
	},
	Drafts: &mailbox.MailboxImpl{
		SerializedMailbox: base.SerializedMailbox{
			Name:       "Drafts",
			Deletable:  false,
			Exportable: false,
			Lifespan:   0,
		},
	},
	WithSpecialChars: &mailbox.MailboxImpl{
		SerializedMailbox: base.SerializedMailbox{
			Name:       "Folder/With/Slashes",
			Deletable:  false,
			Exportable: false,
			Lifespan:   0,
		},
	},
}

// TestSerializedMailboxData provides common test serialized mailbox configurations.
var TestSerializedMailboxData = map[string]base.SerializedMailbox{
	"INBOX": {
		Name:       "INBOX",
		Deletable:  false,
		Exportable: true,
		Lifespan:   30,
	},
	"Sent": {
		Name:       "Sent",
		Deletable:  true,
		Exportable: true,
		Lifespan:   90,
	},
	"Drafts": {
		Name:       "Drafts",
		Deletable:  false,
		Exportable: false,
		Lifespan:   0,
	},
	"Folder/With/Slashes": {
		Name:       "Folder/With/Slashes",
		Deletable:  false,
		Exportable: false,
		Lifespan:   0,
	},
	"Folder With Spaces": {
		Name:       "Folder With Spaces",
		Deletable:  true,
		Exportable: true,
		Lifespan:   60,
	},
}
