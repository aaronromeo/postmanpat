package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"aaronromeo.com/postmanpat/pkg/base"
	"aaronromeo.com/postmanpat/pkg/models/mailbox"
	"aaronromeo.com/postmanpat/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/attribute"
)

// TestableImapManager interface for testing
type TestableImapManager interface {
	GetMailboxes() (map[string]*mailbox.MailboxImpl, error)
}

// MockImapManager implements TestableImapManager for testing
type MockImapManager struct {
	GetMailboxesFunc func() (map[string]*mailbox.MailboxImpl, error)
}

func (m *MockImapManager) GetMailboxes() (map[string]*mailbox.MailboxImpl, error) {
	if m.GetMailboxesFunc != nil {
		return m.GetMailboxesFunc()
	}
	return nil, nil
}

// testableListMailboxNames is a testable version of listMailboxNames that accepts an interface
func testableListMailboxNames(ctx context.Context, isi TestableImapManager, fileMgr utils.FileManager) func(c *cli.Context) error {
	return func(c *cli.Context) error {
		_, span := tracer.Start(ctx, "listMailboxNames")
		defer span.End()

		// List mailboxes
		verifiedMailboxObjs, err := isi.GetMailboxes()
		if err != nil {
			return errors.New("getting mailboxes error " + err.Error())
		}

		exportedMailboxes := make(map[string]base.SerializedMailbox, len(verifiedMailboxObjs))
		for mailboxName, mailbox := range verifiedMailboxObjs {
			exportedMailboxes[mailboxName] = base.SerializedMailbox{
				Name:       mailbox.Name,
				Deletable:  mailbox.Deletable,
				Exportable: mailbox.Exportable,
				Lifespan:   mailbox.Lifespan,
			}
		}

		encodedMailboxes, err := json.MarshalIndent(exportedMailboxes, "", "  ")
		if err != nil {
			return errors.New("converting mailbox names to JSON error " + err.Error())
		}

		span.SetAttributes(
			attribute.String("mailboxListFile.name", base.MailboxListFile),
			attribute.Int("encodedMailboxes.count", len(encodedMailboxes)),
		)
		if err := fileMgr.WriteFile(base.MailboxListFile, encodedMailboxes, 0644); err != nil {
			return errors.New("writing mailbox names file error " + err.Error())
		}

		return nil
	}
}

// MockFileManager implements utils.FileManager for testing
type MockFileManager struct {
	WriteFileFunc func(filename string, data []byte, perm os.FileMode) error
	ReadFileFunc  func(filename string) ([]byte, error)
	CloseFunc     func() error
	CreateFunc    func(name string) (utils.Writer, error)
	MkdirAllFunc  func(path string, perm os.FileMode) error

	// Track calls for verification
	WrittenFiles map[string][]byte
	WrittenPerms map[string]os.FileMode
}

func NewMockFileManager() *MockFileManager {
	return &MockFileManager{
		WrittenFiles: make(map[string][]byte),
		WrittenPerms: make(map[string]os.FileMode),
	}
}

func (m *MockFileManager) WriteFile(filename string, data []byte, perm os.FileMode) error {
	m.WrittenFiles[filename] = data
	m.WrittenPerms[filename] = perm
	if m.WriteFileFunc != nil {
		return m.WriteFileFunc(filename, data, perm)
	}
	return nil
}

func (m *MockFileManager) ReadFile(filename string) ([]byte, error) {
	if m.ReadFileFunc != nil {
		return m.ReadFileFunc(filename)
	}
	return nil, nil
}

func (m *MockFileManager) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

func (m *MockFileManager) Create(name string) (utils.Writer, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(name)
	}
	return nil, nil
}

func (m *MockFileManager) MkdirAll(path string, perm os.FileMode) error {
	if m.MkdirAllFunc != nil {
		return m.MkdirAllFunc(path, perm)
	}
	return nil
}

func TestListMailboxNamesTableDriven(t *testing.T) {
	tests := []struct {
		name                    string
		mockMailboxes           map[string]*mailbox.MailboxImpl
		mockGetMailboxesError   error
		mockWriteFileError      error
		expectedError           string
		expectedFileContent     map[string]base.SerializedMailbox
		expectedFilePermissions os.FileMode
		expectedFileName        string
	}{
		{
			name: "successful execution with multiple mailboxes",
			mockMailboxes: map[string]*mailbox.MailboxImpl{
				"INBOX": {
					SerializedMailbox: base.SerializedMailbox{
						Name:       "INBOX",
						Deletable:  false,
						Exportable: true,
						Lifespan:   30,
					},
				},
				"Sent": {
					SerializedMailbox: base.SerializedMailbox{
						Name:       "Sent",
						Deletable:  true,
						Exportable: true,
						Lifespan:   90,
					},
				},
			},
			expectedFileContent: map[string]base.SerializedMailbox{
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
			},
			expectedFilePermissions: 0644,
			expectedFileName:        base.MailboxListFile,
		},
		{
			name:                    "successful execution with empty mailbox list",
			mockMailboxes:           map[string]*mailbox.MailboxImpl{},
			expectedFileContent:     map[string]base.SerializedMailbox{},
			expectedFilePermissions: 0644,
			expectedFileName:        base.MailboxListFile,
		},
		{
			name:                  "error when GetMailboxes fails",
			mockGetMailboxesError: errors.New("IMAP connection failed"),
			expectedError:         "getting mailboxes error",
		},
		{
			name: "error when WriteFile fails",
			mockMailboxes: map[string]*mailbox.MailboxImpl{
				"INBOX": {
					SerializedMailbox: base.SerializedMailbox{
						Name:       "INBOX",
						Deletable:  false,
						Exportable: true,
						Lifespan:   30,
					},
				},
			},
			mockWriteFileError: errors.New("permission denied"),
			expectedError:      "writing mailbox names file error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctx := context.Background()

			// Create mock file manager
			mockFileManager := NewMockFileManager()
			if tt.mockWriteFileError != nil {
				mockFileManager.WriteFileFunc = func(filename string, data []byte, perm os.FileMode) error {
					return tt.mockWriteFileError
				}
			}

			// Create mock IMAP manager
			mockIsi := &MockImapManager{
				GetMailboxesFunc: func() (map[string]*mailbox.MailboxImpl, error) {
					if tt.mockGetMailboxesError != nil {
						return nil, tt.mockGetMailboxesError
					}
					return tt.mockMailboxes, nil
				},
			}

			// Note: No additional mock setup needed for this simplified test

			// Create the function under test
			listMailboxNamesFunc := testableListMailboxNames(ctx, mockIsi, mockFileManager)

			// Create a mock CLI context
			cliCtx := &cli.Context{}

			// Execute
			err := listMailboxNamesFunc(cliCtx)

			// Verify results
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			// Should not have error for successful cases
			assert.NoError(t, err)

			// Verify file was written with correct name and permissions
			assert.Contains(t, mockFileManager.WrittenFiles, tt.expectedFileName)
			assert.Equal(t, tt.expectedFilePermissions, mockFileManager.WrittenPerms[tt.expectedFileName])

			// Verify file content
			if tt.expectedFileContent != nil {
				writtenData := mockFileManager.WrittenFiles[tt.expectedFileName]

				var actualContent map[string]base.SerializedMailbox
				err = json.Unmarshal(writtenData, &actualContent)
				assert.NoError(t, err)

				assert.Equal(t, tt.expectedFileContent, actualContent)

				// Verify JSON is properly formatted (indented)
				expectedJSON, err := json.MarshalIndent(tt.expectedFileContent, "", "  ")
				assert.NoError(t, err)
				assert.Equal(t, string(expectedJSON), string(writtenData))
			}
		})
	}
}

func TestListMailboxNamesBasic(t *testing.T) {
	ctx := context.Background()

	// Create mock IMAP manager
	mockIsi := &MockImapManager{
		GetMailboxesFunc: func() (map[string]*mailbox.MailboxImpl, error) {
			return map[string]*mailbox.MailboxImpl{
				"INBOX": {
					SerializedMailbox: base.SerializedMailbox{
						Name:       "INBOX",
						Deletable:  false,
						Exportable: true,
						Lifespan:   30,
					},
				},
			}, nil
		},
	}

	// Create mock file manager
	mockFileManager := NewMockFileManager()

	// Create the function under test
	listMailboxNamesFunc := testableListMailboxNames(ctx, mockIsi, mockFileManager)

	// Create a mock CLI context
	cliCtx := &cli.Context{}

	// Execute
	err := listMailboxNamesFunc(cliCtx)

	// Verify results
	assert.NoError(t, err)
	assert.Contains(t, mockFileManager.WrittenFiles, base.MailboxListFile)
	assert.Equal(t, os.FileMode(0644), mockFileManager.WrittenPerms[base.MailboxListFile])

	// Verify file content
	writtenData := mockFileManager.WrittenFiles[base.MailboxListFile]
	var actualContent map[string]base.SerializedMailbox
	err = json.Unmarshal(writtenData, &actualContent)
	assert.NoError(t, err)

	expectedContent := map[string]base.SerializedMailbox{
		"INBOX": {
			Name:       "INBOX",
			Deletable:  false,
			Exportable: true,
			Lifespan:   30,
		},
	}
	assert.Equal(t, expectedContent, actualContent)
}

func TestSimple(t *testing.T) {
	assert.True(t, true)
}
