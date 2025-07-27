package services

import (
	"context"
	"errors"
	"testing"

	"aaronromeo.com/postmanpat/pkg/mock"
	"aaronromeo.com/postmanpat/pkg/models/mailbox"
	"aaronromeo.com/postmanpat/pkg/testutil"
	"github.com/stretchr/testify/assert"
)

func TestMailboxService_ProcessMailboxes(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func() []*testutil.MockMailbox
		expectedError string
		expectedCalls map[string]bool
	}{
		{
			name: "successful processing of multiple mailboxes",
			setupMocks: func() []*testutil.MockMailbox {
				mock1 := testutil.NewMockMailbox()
				mock2 := testutil.NewMockMailbox()
				return []*testutil.MockMailbox{mock1, mock2}
			},
			expectedCalls: map[string]bool{
				"ProcessMailbox": true,
			},
		},
		{
			name: "error during mailbox processing",
			setupMocks: func() []*testutil.MockMailbox {
				mock1 := testutil.NewMockMailbox()
				mock1.ProcessMailboxFunc = func(ctx context.Context) error {
					return errors.New("processing failed")
				}
				return []*testutil.MockMailbox{mock1}
			},
			expectedError: "processing failed",
			expectedCalls: map[string]bool{
				"ProcessMailbox": true,
			},
		},
		{
			name: "empty mailbox list",
			setupMocks: func() []*testutil.MockMailbox {
				return []*testutil.MockMailbox{}
			},
			expectedCalls: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			logger := mock.SetupLogger(t)
			service := NewMailboxService(logger)
			ctx := context.Background()

			mocks := tt.setupMocks()

			// Convert to mailbox interface slice
			mailboxes := make([]mailbox.Mailbox, len(mocks))
			for i, mock := range mocks {
				mailboxes[i] = mock
			}

			// Execute
			err := service.ProcessMailboxes(ctx, mailboxes)

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			// Verify method calls
			for _, mock := range mocks {
				if tt.expectedCalls["ProcessMailbox"] {
					assert.True(t, mock.ProcessMailboxCalled, "ProcessMailbox should have been called")
				}
			}
		})
	}
}

func TestMailboxService_ReapMailbox(t *testing.T) {
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
			service := NewMailboxService(logger)
			ctx := context.Background()
			mockMailbox := tt.setupMock()

			// Execute
			err := service.ReapMailbox(ctx, mockMailbox)

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			assert.True(t, mockMailbox.ReapCalled, "Reap should have been called")
		})
	}
}

func TestMailboxService_ExportMailbox(t *testing.T) {
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
			service := NewMailboxService(logger)
			ctx := context.Background()
			mockMailbox := tt.setupMock()

			// Execute
			err := service.ExportMailbox(ctx, mockMailbox)

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			assert.True(t, mockMailbox.ExportAndDeleteMessagesCalled, "ExportAndDeleteMessages should have been called")
		})
	}
}

func TestMailboxService_DeleteMailbox(t *testing.T) {
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
			service := NewMailboxService(logger)
			ctx := context.Background()
			mockMailbox := tt.setupMock()

			// Execute
			err := service.DeleteMailbox(ctx, mockMailbox)

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			assert.True(t, mockMailbox.DeleteMessagesCalled, "DeleteMessages should have been called")
		})
	}
}
