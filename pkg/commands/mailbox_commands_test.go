package commands

import (
	"context"
	"errors"
	"testing"

	"aaronromeo.com/postmanpat/pkg/mock"
	"aaronromeo.com/postmanpat/pkg/testutil"
	"github.com/stretchr/testify/assert"
)

func TestReapCommand(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func() *testutil.MockMailbox
		expectedError string
	}{
		{
			name: "successful reap",
			setupMock: func() *testutil.MockMailbox {
				return testutil.NewMockMailbox()
			},
		},
		{
			name: "reap error",
			setupMock: func() *testutil.MockMailbox {
				mock := testutil.NewMockMailbox()
				mock.ReapFunc = func() error {
					return errors.New("reap failed")
				}
				return mock
			},
			expectedError: "reap failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			logger := mock.SetupLogger(t)
			cmd := NewReapCommand(logger)
			ctx := context.Background()
			mockMailbox := tt.setupMock()

			// Execute
			err := cmd.Execute(ctx, mockMailbox)

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			assert.True(t, mockMailbox.ReapCalled, "Reap should have been called")
			assert.Equal(t, "reap", cmd.GetName())
			assert.Equal(t, "Reaps messages from the mailbox", cmd.GetDescription())
		})
	}
}

func TestExportCommand(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func() *testutil.MockMailbox
		expectedError string
	}{
		{
			name: "successful export",
			setupMock: func() *testutil.MockMailbox {
				return testutil.NewMockMailbox()
			},
		},
		{
			name: "export error",
			setupMock: func() *testutil.MockMailbox {
				mock := testutil.NewMockMailbox()
				mock.ExportAndDeleteMessagesFunc = func() error {
					return errors.New("export failed")
				}
				return mock
			},
			expectedError: "export failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			logger := mock.SetupLogger(t)
			cmd := NewExportCommand(logger)
			ctx := context.Background()
			mockMailbox := tt.setupMock()

			// Execute
			err := cmd.Execute(ctx, mockMailbox)

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			assert.True(t, mockMailbox.ExportAndDeleteMessagesCalled, "ExportAndDeleteMessages should have been called")
			assert.Equal(t, "export", cmd.GetName())
			assert.Equal(t, "Exports and deletes messages from the mailbox", cmd.GetDescription())
		})
	}
}

func TestDeleteCommand(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func() *testutil.MockMailbox
		expectedError string
	}{
		{
			name: "successful delete",
			setupMock: func() *testutil.MockMailbox {
				return testutil.NewMockMailbox()
			},
		},
		{
			name: "delete error",
			setupMock: func() *testutil.MockMailbox {
				mock := testutil.NewMockMailbox()
				mock.DeleteMessagesFunc = func() error {
					return errors.New("delete failed")
				}
				return mock
			},
			expectedError: "delete failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			logger := mock.SetupLogger(t)
			cmd := NewDeleteCommand(logger)
			ctx := context.Background()
			mockMailbox := tt.setupMock()

			// Execute
			err := cmd.Execute(ctx, mockMailbox)

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			assert.True(t, mockMailbox.DeleteMessagesCalled, "DeleteMessages should have been called")
			assert.Equal(t, "delete", cmd.GetName())
			assert.Equal(t, "Deletes messages from the mailbox without exporting", cmd.GetDescription())
		})
	}
}

func TestProcessCommand(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func() *testutil.MockMailbox
		expectedError string
	}{
		{
			name: "successful process",
			setupMock: func() *testutil.MockMailbox {
				return testutil.NewMockMailbox()
			},
		},
		{
			name: "process error",
			setupMock: func() *testutil.MockMailbox {
				mock := testutil.NewMockMailbox()
				mock.ProcessMailboxFunc = func(ctx context.Context) error {
					return errors.New("process failed")
				}
				return mock
			},
			expectedError: "process failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			logger := mock.SetupLogger(t)
			cmd := NewProcessCommand(logger)
			ctx := context.Background()
			mockMailbox := tt.setupMock()

			// Execute
			err := cmd.Execute(ctx, mockMailbox)

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			assert.True(t, mockMailbox.ProcessMailboxCalled, "ProcessMailbox should have been called")
			assert.Equal(t, "process", cmd.GetName())
			assert.Equal(t, "Processes the mailbox according to its configuration", cmd.GetDescription())
		})
	}
}

func TestCommandExecutor(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func() *testutil.MockMailbox
		setupCommand  func() MailboxCommand
		expectedError string
	}{
		{
			name: "successful command execution",
			setupMock: func() *testutil.MockMailbox {
				return testutil.NewMockMailbox()
			},
			setupCommand: func() MailboxCommand {
				logger := mock.SetupLogger(t)
				return NewReapCommand(logger)
			},
		},
		{
			name: "command execution error",
			setupMock: func() *testutil.MockMailbox {
				mock := testutil.NewMockMailbox()
				mock.ReapFunc = func() error {
					return errors.New("command failed")
				}
				return mock
			},
			setupCommand: func() MailboxCommand {
				logger := mock.SetupLogger(t)
				return NewReapCommand(logger)
			},
			expectedError: "command failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			logger := mock.SetupLogger(t)
			executor := NewCommandExecutor(logger)
			ctx := context.Background()
			mockMailbox := tt.setupMock()
			command := tt.setupCommand()

			// Execute
			err := executor.ExecuteCommand(ctx, command, mockMailbox)

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCommandExecutor_ExecuteCommands(t *testing.T) {
	// Setup
	logger := mock.SetupLogger(t)
	executor := NewCommandExecutor(logger)
	ctx := context.Background()
	mockMailbox := testutil.NewMockMailbox()

	commands := []MailboxCommand{
		NewReapCommand(logger),
		NewExportCommand(logger),
	}

	// Execute
	err := executor.ExecuteCommands(ctx, commands, mockMailbox)

	// Verify
	assert.NoError(t, err)
	assert.True(t, mockMailbox.ReapCalled, "Reap should have been called")
	assert.True(t, mockMailbox.ExportAndDeleteMessagesCalled, "ExportAndDeleteMessages should have been called")
}
