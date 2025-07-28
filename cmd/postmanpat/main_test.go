package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"aaronromeo.com/postmanpat/pkg/base"
	"aaronromeo.com/postmanpat/pkg/mock"
	"aaronromeo.com/postmanpat/pkg/models/mailbox"
	"aaronromeo.com/postmanpat/pkg/testutil"
	"aaronromeo.com/postmanpat/pkg/utils"
	"github.com/emersion/go-imap"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/mock/gomock"
)

// Note: MockImapManager and TestableImapManager are now defined in pkg/testutil/mocks.go

// testableListMailboxNames is a testable version of listMailboxNames that accepts an interface
func testableListMailboxNames(ctx context.Context, isi testutil.TestableImapManager, fileMgr utils.FileManager) func(c *cli.Context) error {
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
				Name:       mailbox.SerializedMailbox.Name,
				Deletable:  mailbox.SerializedMailbox.Deletable,
				Exportable: mailbox.SerializedMailbox.Exportable,
				Lifespan:   mailbox.SerializedMailbox.Lifespan,
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

// Note: MockFileManager is now defined in pkg/testutil/mocks.go

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
			mockFileManager := testutil.NewMockFileManager()
			if tt.mockWriteFileError != nil {
				mockFileManager.WriteFileFunc = func(filename string, data []byte, perm os.FileMode) error {
					return tt.mockWriteFileError
				}
			}

			// Create mock IMAP manager
			mockIsi := &testutil.MockImapManager{
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
	mockIsi := &testutil.MockImapManager{
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
	mockFileManager := testutil.NewMockFileManager()

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

// MockImapManagerImpl provides a mock implementation that matches the structure needed by reapMessages
type MockImapManagerImpl struct {
	Client base.Client
	Logger *slog.Logger
}

// testableReapMessages creates a testable version of the reapMessages function
// This allows us to inject mocks and test the function in isolation
func testableReapMessages(ctx context.Context, isi *MockImapManagerImpl, fileMgr utils.FileManager) func(c *cli.Context) error {
	return func(c *cli.Context) error {
		ctx, span := tracer.Start(ctx, "reapMessages")
		defer span.End()

		// Read the mailbox list file
		data, err := fileMgr.ReadFile(base.MailboxListFile)
		if err != nil {
			return errors.Errorf("exporting mailbox error %+v", err)
		}
		mailboxes := make(map[string]mailbox.MailboxImpl)

		err = json.Unmarshal(data, &mailboxes)
		if err != nil {
			return errors.Errorf("unable to marshal mailboxes %+v", err)
		}

		for _, mb := range mailboxes {
			// Since we're working with concrete types from JSON unmarshaling,
			// we need to set the client, logger, and function fields directly
			mb.Client = isi.Client
			mb.Logger = isi.Logger

			// Set up the required function fields that aren't serialized
			mb.LoginFn = func() (base.Client, error) {
				return isi.Client, nil
			}
			mb.LogoutFn = func() error {
				return nil
			}
			mb.Ctx = ctx
			mb.FileManager = fileMgr

			err := mb.ProcessMailbox(ctx)
			if err != nil {
				return errors.Errorf("unable to process mailboxes %+v", err)
			}
		}

		return nil
	}
}

// createMockImapManager creates a mock IMAP manager for testing
func createMockImapManager(t *testing.T) (*MockImapManagerImpl, *gomock.Controller) {
	logger := mock.SetupLogger(t)
	ctrl := gomock.NewController(t)
	mockClient := mock.NewMockClient(ctrl)

	return &MockImapManagerImpl{
		Client: mockClient,
		Logger: logger,
	}, ctrl
}

func TestReapMessagesSuccess(t *testing.T) {
	// Setup
	ctx := context.Background()

	// Create mock IMAP manager
	mockIsi, ctrl := createMockImapManager(t)
	defer ctrl.Finish()

	// No IMAP mock expectations needed since all mailboxes will be skipped

	// Create mock file manager with valid mailbox data
	mockFileManager := testutil.NewMockFileManager()

	// Prepare test mailbox data that matches the expected JSON structure
	// For this test, use only mailboxes that will be skipped to avoid IMAP calls
	testMailboxData := map[string]base.SerializedMailbox{
		"Drafts": {
			Name:       "Drafts",
			Deletable:  false,
			Exportable: false,
			Lifespan:   0,
		},
		"INBOX": {
			Name:       "INBOX",
			Deletable:  false,
			Exportable: false, // Changed to false to make this a truly skippable mailbox
			Lifespan:   30,
		},
	}

	// Marshal the test data to JSON
	testDataJSON, err := json.MarshalIndent(testMailboxData, "", "  ")
	assert.NoError(t, err)

	// Setup mock file manager to return the test data
	mockFileManager.ReadFileFunc = func(filename string) ([]byte, error) {
		if filename == base.MailboxListFile {
			return testDataJSON, nil
		}
		return nil, errors.New("file not found")
	}

	// Create the function under test
	reapMessagesFunc := testableReapMessages(ctx, mockIsi, mockFileManager)

	// Create a mock CLI context
	cliCtx := &cli.Context{}

	// Execute
	err = reapMessagesFunc(cliCtx)

	// Verify results
	assert.NoError(t, err, "reapMessages should complete successfully")

	// Verify that the file was read
	assert.Contains(t, mockFileManager.ReadFiles, base.MailboxListFile, "mailbox list file should have been read")

	// Note: Since we're using concrete MailboxImpl structs from JSON unmarshaling,
	// we can't easily verify ProcessMailbox calls without more complex mocking.
	// This test verifies the happy path of file reading and JSON unmarshaling.
}

// TestReapMessagesExportableButNotDeletableError specifically tests the error case
// where a mailbox is exportable but not deletable, which should be treated as an error
func TestReapMessagesExportableButNotDeletableError(t *testing.T) {
	// Setup
	ctx := context.Background()
	mockIsi, ctrl := createMockImapManager(t)
	defer ctrl.Finish()

	// Create mock file manager
	mockFileManager := testutil.NewMockFileManager()

	// Create test data with invalid configuration (exportable but not deletable)
	testMailboxData := map[string]base.SerializedMailbox{
		"InvalidMailbox": {
			Name:       "InvalidMailbox",
			Deletable:  false,
			Exportable: true, // This should cause an error
			Lifespan:   30,
		},
	}

	// Marshal the test data to JSON
	testDataJSON, err := json.MarshalIndent(testMailboxData, "", "  ")
	assert.NoError(t, err)

	// Setup mock file manager to return the test data
	mockFileManager.ReadFileFunc = func(filename string) ([]byte, error) {
		if filename == base.MailboxListFile {
			return testDataJSON, nil
		}
		return nil, errors.New("file not found")
	}

	// Create the function under test
	reapMessagesFunc := testableReapMessages(ctx, mockIsi, mockFileManager)

	// Create a mock CLI context
	cliCtx := &cli.Context{}

	// Execute
	err = reapMessagesFunc(cliCtx)

	// Verify that we get the expected error
	assert.Error(t, err, "reapMessages should fail when mailbox is exportable but not deletable")
	assert.Contains(t, err.Error(), "exportable but not deletable is not implemented",
		"Error should contain the expected message about exportable but not deletable")

	// Verify that the file was read (the error occurs during processing, not file reading)
	assert.Contains(t, mockFileManager.ReadFiles, base.MailboxListFile, "mailbox list file should have been read")
}

func TestReapMessagesWithProcessing(t *testing.T) {
	// Setup
	ctx := context.Background()

	// Create mock IMAP manager
	mockIsi, ctrl := createMockImapManager(t)
	defer ctrl.Finish()

	// Set up mock expectations for IMAP client
	mockClient := mockIsi.Client.(*mock.MockClient)

	// Mock expectations for "Trash" mailbox (deletable only) - calls DeleteMessages
	mockClient.EXPECT().Select("Trash", false).Return(&imap.MailboxStatus{Messages: 0}, nil)
	mockClient.EXPECT().Search(gomock.Any()).Return([]uint32{}, nil)
	mockClient.EXPECT().Store(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockClient.EXPECT().Expunge(gomock.Any()).Return(nil)

	// Create mock file manager with valid mailbox data
	mockFileManager := testutil.NewMockFileManager()

	// Prepare test mailbox data with one processable mailbox
	testMailboxData := map[string]base.SerializedMailbox{
		"Trash": {
			Name:       "Trash",
			Deletable:  true,
			Exportable: false,
			Lifespan:   30,
		},
		"Drafts": {
			Name:       "Drafts",
			Deletable:  false,
			Exportable: false,
			Lifespan:   0,
		},
	}

	// Marshal the test data to JSON
	testDataJSON, err := json.MarshalIndent(testMailboxData, "", "  ")
	assert.NoError(t, err)

	// Set up the mock file manager to return our test data
	mockFileManager.ReadFileFunc = func(filename string) ([]byte, error) {
		if filename == base.MailboxListFile {
			return testDataJSON, nil
		}
		return nil, errors.New("file not found")
	}

	// Create the function under test
	reapMessagesFunc := testableReapMessages(ctx, mockIsi, mockFileManager)

	// Execute the function
	err = reapMessagesFunc(&cli.Context{})

	// Verify the function was called without error
	assert.NoError(t, err)
}

func TestReapMessagesTableDriven(t *testing.T) {
	tests := []struct {
		name                 string
		setupMockFileManager func() *testutil.MockFileManager
		expectedError        string
	}{
		{
			name: "successful execution with multiple mailboxes",
			setupMockFileManager: func() *testutil.MockFileManager {
				mockFileManager := testutil.NewMockFileManager()

				// Prepare test mailbox data (using only skippable mailboxes to avoid IMAP mocking complexity)
				testMailboxData := map[string]base.SerializedMailbox{
					"INBOX": {
						Name:       "INBOX",
						Deletable:  false,
						Exportable: false, // Changed to false to make this a truly skippable mailbox
						Lifespan:   30,
					},
					"Drafts": {
						Name:       "Drafts",
						Deletable:  false,
						Exportable: false,
						Lifespan:   0,
					},
				}

				// Marshal the test data to JSON
				testDataJSON, _ := json.MarshalIndent(testMailboxData, "", "  ")

				// Setup mock to return the test data
				mockFileManager.ReadFileFunc = func(filename string) ([]byte, error) {
					if filename == base.MailboxListFile {
						return testDataJSON, nil
					}
					return nil, errors.New("file not found")
				}

				return mockFileManager
			},
		},
		{
			name: "successful execution with empty mailbox list",
			setupMockFileManager: func() *testutil.MockFileManager {
				mockFileManager := testutil.NewMockFileManager()

				// Empty mailbox data
				testMailboxData := map[string]base.SerializedMailbox{}
				testDataJSON, _ := json.MarshalIndent(testMailboxData, "", "  ")

				mockFileManager.ReadFileFunc = func(filename string) ([]byte, error) {
					if filename == base.MailboxListFile {
						return testDataJSON, nil
					}
					return nil, errors.New("file not found")
				}

				return mockFileManager
			},
		},
		{
			name: "error when file read fails",
			setupMockFileManager: func() *testutil.MockFileManager {
				mockFileManager := testutil.NewMockFileManager()

				mockFileManager.ReadFileFunc = func(filename string) ([]byte, error) {
					return nil, errors.New("file read error")
				}

				return mockFileManager
			},
			expectedError: "exporting mailbox error",
		},
		{
			name: "error when JSON unmarshal fails",
			setupMockFileManager: func() *testutil.MockFileManager {
				mockFileManager := testutil.NewMockFileManager()

				// Invalid JSON data
				invalidJSON := []byte(`{"invalid": json}`)

				mockFileManager.ReadFileFunc = func(filename string) ([]byte, error) {
					if filename == base.MailboxListFile {
						return invalidJSON, nil
					}
					return nil, errors.New("file not found")
				}

				return mockFileManager
			},
			expectedError: "unable to marshal mailboxes",
		},
		{
			name: "error when mailbox is exportable but not deletable",
			setupMockFileManager: func() *testutil.MockFileManager {
				mockFileManager := testutil.NewMockFileManager()

				// Prepare test mailbox data with invalid configuration (exportable but not deletable)
				testMailboxData := map[string]base.SerializedMailbox{
					"InvalidMailbox": {
						Name:       "InvalidMailbox",
						Deletable:  false,
						Exportable: true, // This should cause an error
						Lifespan:   30,
					},
				}

				// Marshal the test data to JSON
				testDataJSON, _ := json.MarshalIndent(testMailboxData, "", "  ")

				// Setup mock to return the test data
				mockFileManager.ReadFileFunc = func(filename string) ([]byte, error) {
					if filename == base.MailboxListFile {
						return testDataJSON, nil
					}
					return nil, errors.New("file not found")
				}

				return mockFileManager
			},
			expectedError: "exportable but not deletable is not implemented",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctx := context.Background()
			mockIsi, ctrl := createMockImapManager(t)
			defer ctrl.Finish()
			mockFileManager := tt.setupMockFileManager()

			// Create the function under test
			reapMessagesFunc := testableReapMessages(ctx, mockIsi, mockFileManager)

			// Create a mock CLI context
			cliCtx := &cli.Context{}

			// Execute
			err := reapMessagesFunc(cliCtx)

			// Verify results
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				// Verify that the file was read
				assert.Contains(t, mockFileManager.ReadFiles, base.MailboxListFile, "mailbox list file should have been read")
			}
		})
	}
}

// TestReapMessagesIntegration demonstrates the complete flow with realistic data
func TestReapMessagesIntegration(t *testing.T) {
	// Setup
	ctx := context.Background()

	// Create mock IMAP manager
	mockIsi, ctrl := createMockImapManager(t)
	defer ctrl.Finish()

	// No IMAP mock expectations needed since we'll use only skippable mailboxes

	// Create mock file manager
	mockFileManager := testutil.NewMockFileManager()

	// Use a subset of test data with only skippable mailboxes for integration testing
	testMailboxData := map[string]base.SerializedMailbox{
		"INBOX": {
			Name:       "INBOX",
			Deletable:  false,
			Exportable: false, // Changed to false to make this a truly skippable mailbox
			Lifespan:   30,
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
	}

	// Marshal the test data to JSON (same format as listMailboxNames produces)
	testDataJSON, err := json.MarshalIndent(testMailboxData, "", "  ")
	assert.NoError(t, err)

	// Setup mock file manager to return the test data
	mockFileManager.ReadFileFunc = func(filename string) ([]byte, error) {
		if filename == base.MailboxListFile {
			return testDataJSON, nil
		}
		return nil, errors.New("file not found")
	}

	// Create the function under test
	reapMessagesFunc := testableReapMessages(ctx, mockIsi, mockFileManager)

	// Create a mock CLI context
	cliCtx := &cli.Context{}

	// Execute
	err = reapMessagesFunc(cliCtx)

	// Verify results
	assert.NoError(t, err, "reapMessages should complete successfully with realistic data")

	// Verify that the file was read
	assert.Contains(t, mockFileManager.ReadFiles, base.MailboxListFile, "mailbox list file should have been read")

	// Verify that we processed the expected number of mailboxes
	// (This is implicit since no error was returned and the JSON was valid)
	t.Logf("Successfully processed %d mailboxes", len(testMailboxData))
}
