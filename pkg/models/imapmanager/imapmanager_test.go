package imapmanager

import (
	"context"
	"crypto/tls"
	"testing"

	"aaronromeo.com/postmanpat/pkg/base"
	"aaronromeo.com/postmanpat/pkg/mock"
	"github.com/emersion/go-imap"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestNewImapManager(t *testing.T) {
	logger := mock.SetupLogger(t)
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockClient(ctrl)

	// Test successful creation
	t.Run("Successful Creation", func(t *testing.T) {
		service, err := NewImapManager(
			WithAuth("username", "password"),
			WithClient(mockClient),
			WithLogger(logger),
			WithCtx(ctx),
			WithFileManager(mock.MockFileWriter{}),
		)
		assert.NoError(t, err)
		assert.NotNil(t, service)
		assert.Equal(t, "username", service.Username)
		assert.Equal(t, "password", service.password)
		assert.Equal(t, mockClient, service.Client)
		assert.Equal(t, logger, service.Logger)
		assert.Equal(t, ctx, service.ctx)
	})

	// Test missing username
	t.Run("Missing Username", func(t *testing.T) {
		_, err := NewImapManager(
			WithAuth("", "password"),
			WithClient(mockClient),
			WithLogger(logger),
			WithCtx(ctx),
		)
		assert.Error(t, err)
	})

	// Test missing client
	t.Run("Missing Client", func(t *testing.T) {
		_, err := NewImapManager(
			WithAuth("username", "password"),
			WithLogger(logger),
			WithCtx(ctx),
		)
		assert.Error(t, err)
	})
}

func TestGetMailboxesX(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockClient(ctrl)
	logger := mock.SetupLogger(t)
	ctx := context.Background()

	service, err := NewImapManager(
		WithAuth("foo", "bar"),
		WithClient(mockClient),
		WithLogger(logger),
		WithCtx(ctx),
		WithFileManager(mock.MockFileWriter{}),
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
		State().Return(imap.NotAuthenticatedState)

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

	actual := map[string]base.SerializedMailbox{}
	for _, mb := range result {
		actual[mb.Name], err = mb.Serialize()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Define expected results
	expected := map[string]base.SerializedMailbox{
		"Folder1": {Name: "Folder1", Deletable: false, Exportable: false},
		"Folder2": {Name: "Folder2", Deletable: false, Exportable: false},
		"Folder3": {Name: "Folder3", Deletable: false, Exportable: false},
		"Folder4": {Name: "Folder4", Deletable: false, Exportable: false},
		"Folder5": {Name: "Folder5", Deletable: false, Exportable: false},
	}

	// Check if the results meet the expectations
	assert.Equal(t, expected, actual, "The returned map of mailboxes should match the expected values.")
}

func TestGetMailboxesErrorHandling(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockClient(ctrl)
	logger := mock.SetupLogger(t)
	ctx := context.Background()

	service, err := NewImapManager(
		WithClient(mockClient),
		WithAuth("testuser", "testpass"),
		WithLogger(logger),
		WithCtx(ctx),
		WithFileManager(mock.MockFileWriter{}),
	)
	assert.Nil(t, err, "Setup failed")

	// Setup failing conditions
	mockClient.EXPECT().Login(gomock.Any(), gomock.Any()).Return(nil)
	mockClient.EXPECT().State().Return(imap.NotAuthenticatedState)
	mockClient.EXPECT().List("", "*", gomock.Any()).DoAndReturn(func(_, _ string, ch chan *imap.MailboxInfo) error {
		close(ch) // Ensure the channel is closed even when simulating an error
		return errors.New("failed to list mailboxes")
	})
	mockClient.EXPECT().Logout().Return(nil)

	// Execute the function
	_, err = service.GetMailboxes()
	assert.NotNil(t, err, "Should return an error when listing mailboxes fails")
}

func TestLogin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDailerCallCount := 0
	mockClient := mock.NewMockClient(ctrl)
	mockDialer := func(address string, tlsConfig *tls.Config) (base.Client, error) {
		mockDailerCallCount++
		return mockClient, nil
	}
	logger := mock.SetupLogger(t)
	ctx := context.Background()

	// Setup service
	service, err := NewImapManager(
		WithAuth("testuser", "testpass"),
		WithClient(mockClient),
		WithCtx(ctx),
		WithDialTLS(mockDialer),
		WithLogger(logger),
		WithFileManager(mock.MockFileWriter{}),
	)
	assert.Nil(t, err, "Setup failed")

	// Test cases
	tests := []struct {
		name                    string
		setupMocks              func(*mock.MockClient)
		wantMockDailerCallCount int
		wantErr                 bool
	}{
		{
			name: "Successful login from not authenticated state",
			setupMocks: func(mc *mock.MockClient) {
				mockDailerCallCount = 0
				mc.EXPECT().State().DoAndReturn(func() imap.ConnState {
					return imap.NotAuthenticatedState
				})
				mc.EXPECT().Login("testuser", "testpass").Return(nil)
			},
			wantMockDailerCallCount: 0,
			wantErr:                 false,
		},
		{
			name: "Failed login from not authenticated state",
			setupMocks: func(mc *mock.MockClient) {
				mockDailerCallCount = 0
				mc.EXPECT().State().DoAndReturn(func() imap.ConnState {
					return imap.NotAuthenticatedState
				})
				mc.EXPECT().Login("testuser", "testpass").Return(errors.New("login failed"))
			},
			wantMockDailerCallCount: 0,
			wantErr:                 true,
		},
		{
			name: "Already authenticated",
			setupMocks: func(mc *mock.MockClient) {
				mockDailerCallCount = 0
				mc.EXPECT().State().DoAndReturn(func() imap.ConnState {
					return imap.AuthenticatedState
				})
			},
			wantMockDailerCallCount: 0,
			wantErr:                 false,
		},
		{
			name: "Already selected mailbox",
			setupMocks: func(mc *mock.MockClient) {
				mockDailerCallCount = 0
				mc.EXPECT().State().DoAndReturn(func() imap.ConnState {
					return imap.SelectedState
				})
			},
			wantMockDailerCallCount: 0,
			wantErr:                 false,
		},
		{
			name: "Successful re-login after disconnect",
			setupMocks: func(mc *mock.MockClient) {
				mockDailerCallCount = 0
				mc.EXPECT().State().DoAndReturn(func() imap.ConnState {
					return imap.LogoutState
				})
				mc.EXPECT().Login("testuser", "testpass").Return(nil)
			},
			wantMockDailerCallCount: 1,
			wantErr:                 false,
		},
		{
			name: "Failed re-login after disconnect",
			setupMocks: func(mc *mock.MockClient) {
				mockDailerCallCount = 0
				mc.EXPECT().State().DoAndReturn(func() imap.ConnState {
					return imap.LogoutState
				})
				mc.EXPECT().Login("testuser", "testpass").Return(errors.New("login failed"))
			},
			wantMockDailerCallCount: 1,
			wantErr:                 true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks(mockClient)
			_, err := service.Login()
			if (err != nil) != tt.wantErr {
				t.Errorf("Login() error = %v, wantErr %v", err, tt.wantErr)
			}
			if mockDailerCallCount != tt.wantMockDailerCallCount {
				t.Errorf("Login() mockDailerCallCount = %v, want %v", mockDailerCallCount, tt.wantMockDailerCallCount)
			}
		})
	}
}

func TestLogoutFn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockClient(ctrl)
	logger := mock.SetupLogger(t)
	ctx := context.Background()

	service, err := NewImapManager(
		WithClient(mockClient),
		WithAuth("testuser", "testpass"),
		WithLogger(logger),
		WithCtx(ctx),
		WithFileManager(mock.MockFileWriter{}),
	)
	assert.Nil(t, err, "Setup failed")

	// Expectations
	mockClient.EXPECT().Logout().Return(nil)

	// Execute the logout function returned by LogoutFn
	logoutFunc := service.LogoutFn()
	logoutFunc() // this should call Logout on the client
}
