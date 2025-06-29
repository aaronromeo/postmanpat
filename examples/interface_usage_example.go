// Package examples demonstrates how to use the new interface-based architecture in PostmanPat.
// This example shows the benefits of using interfaces for better testability, maintainability,
// and extensibility.
package main

import (
	"context"
	"log/slog"
	"os"

	"aaronromeo.com/postmanpat/pkg/commands"
	"aaronromeo.com/postmanpat/pkg/models/mailbox"
	"aaronromeo.com/postmanpat/pkg/services"
)

func main() {
	// Setup logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx := context.Background()

	// Example 1: Using Interface-Based Factory
	demonstrateInterfaceFactory(logger)

	// Example 2: Using Service Layer
	demonstrateServiceLayer(ctx, logger)

	// Example 3: Using Command Pattern
	demonstrateCommandPattern(ctx, logger)

	// Example 4: Using Repository Pattern
	demonstrateRepositoryPattern(logger)

	// Example 5: Polymorphic Behavior
	demonstratePolymorphism(ctx, logger)
}

// Example 1: Interface-Based Factory
func demonstrateInterfaceFactory(logger *slog.Logger) {
	logger.Info("=== Example 1: Interface-Based Factory ===")

	// ✅ NEW: Returns interface for better abstraction
	mb, err := mailbox.NewMailbox(
	// ... options would go here in real usage
	)
	if err != nil {
		logger.Error("Failed to create mailbox", slog.String("error", err.Error()))
		return
	}

	// Can use interface methods
	_ = mb // mb is of type mailbox.Mailbox interface

	// ✅ NEW: If you need concrete type for specific operations
	mbImpl, err := mailbox.NewMailboxImpl(
	// ... options would go here in real usage
	)
	if err != nil {
		logger.Error("Failed to create mailbox impl", slog.String("error", err.Error()))
		return
	}

	// Can access implementation-specific fields
	_ = mbImpl.Client // Only available on concrete type

	logger.Info("Interface factory demonstration completed")
}

// Example 2: Service Layer
func demonstrateServiceLayer(ctx context.Context, logger *slog.Logger) {
	logger.Info("=== Example 2: Service Layer ===")

	// Create service
	mailboxService := services.NewMailboxService(logger)

	// Create mock mailboxes for demonstration
	// In real usage, these would come from IMAP manager
	var mailboxes []mailbox.Mailbox
	// mailboxes = append(mailboxes, ...) // Add real mailboxes

	// ✅ NEW: Process mailboxes using service layer
	if err := mailboxService.ProcessMailboxes(ctx, mailboxes); err != nil {
		logger.Error("Failed to process mailboxes", slog.String("error", err.Error()))
		return
	}

	logger.Info("Service layer demonstration completed")
}

// Example 3: Command Pattern
func demonstrateCommandPattern(ctx context.Context, logger *slog.Logger) {
	logger.Info("=== Example 3: Command Pattern ===")

	// Create command executor
	_ = commands.NewCommandExecutor(logger)

	// Create different commands
	_ = commands.NewReapCommand(logger)
	_ = commands.NewExportCommand(logger)
	_ = commands.NewDeleteCommand(logger)
	_ = commands.NewProcessCommand(logger)

	// Mock mailbox for demonstration
	// var mb mailbox.Mailbox = ... // Real mailbox would go here

	// ✅ NEW: Execute individual commands
	// err := executor.ExecuteCommand(ctx, reapCmd, mb)
	// err = executor.ExecuteCommand(ctx, exportCmd, mb)

	// ✅ NEW: Execute multiple commands in sequence
	// commandSequence := []commands.MailboxCommand{reapCmd, exportCmd, deleteCmd}
	// err = executor.ExecuteCommands(ctx, commandSequence, mb)

	// ✅ NEW: Execute command on multiple mailboxes
	// var mailboxes []mailbox.Mailbox
	// err = executor.ExecuteCommandOnMailboxes(ctx, processCmd, mailboxes)

	logger.Info("Command pattern demonstration completed")
}

// Example 4: Repository Pattern
func demonstrateRepositoryPattern(logger *slog.Logger) {
	logger.Info("=== Example 4: Repository Pattern ===")

	// In real usage, you would create actual IMAP manager and file manager
	// var imapMgr imapmanager.ImapManager = ...
	// var fileMgr utils.FileManager = ...

	// Create repository
	// repo := repositories.NewImapMailboxRepository(imapMgr, fileMgr, logger)

	// ✅ NEW: Get mailboxes as interfaces
	// mailboxes, err := repo.GetMailboxes()
	// if err != nil {
	//     logger.Error("Failed to get mailboxes", slog.String("error", err.Error()))
	//     return
	// }

	// ✅ NEW: Save mailboxes
	// err = repo.SaveMailboxes(mailboxes)
	// if err != nil {
	//     logger.Error("Failed to save mailboxes", slog.String("error", err.Error()))
	//     return
	// }

	logger.Info("Repository pattern demonstration completed")
}

// Example 5: Polymorphic Behavior
func demonstratePolymorphism(ctx context.Context, logger *slog.Logger) {
	logger.Info("=== Example 5: Polymorphic Behavior ===")

	// ✅ NEW: Different implementations can be used interchangeably
	var mailboxes []mailbox.Mailbox

	// Could add different types of mailboxes:
	// - IMAP mailboxes
	// - Mock mailboxes for testing
	// - Future implementations (POP3, local files, etc.)

	// All can be processed the same way
	for _, mb := range mailboxes {
		// Polymorphic behavior - each implementation handles this differently
		if err := mb.ProcessMailbox(ctx); err != nil {
			logger.Error("Failed to process mailbox", slog.String("error", err.Error()))
			continue
		}
	}

	logger.Info("Polymorphism demonstration completed")
}

// Example 6: Testing Benefits
func demonstrateTestingBenefits() {
	// This would be in a test file, showing how interfaces improve testing

	/*
		func TestMailboxProcessing(t *testing.T) {
			// ✅ NEW: Easy to create mocks
			mockMailbox := testutil.NewMockMailbox()
			mockMailbox.ProcessMailboxFunc = func(ctx context.Context) error {
				return nil // or test-specific behavior
			}

			// ✅ NEW: Test with interface
			service := services.NewMailboxService(logger)
			err := service.ProcessMailboxes(ctx, []mailbox.Mailbox{mockMailbox})

			// ✅ NEW: Verify behavior
			assert.NoError(t, err)
			assert.True(t, mockMailbox.ProcessMailboxCalled)
		}
	*/
}

// Example 7: Dependency Injection
func demonstrateDependencyInjection() {
	// This shows how interfaces enable dependency injection

	/*
		type Application struct {
			mailboxService services.MailboxService
			mailboxRepo    repositories.MailboxRepository
			commandExecutor *commands.CommandExecutor
		}

		func NewApplication(
			mailboxService services.MailboxService,
			mailboxRepo repositories.MailboxRepository,
			commandExecutor *commands.CommandExecutor,
		) *Application {
			return &Application{
				mailboxService: mailboxService,
				mailboxRepo: mailboxRepo,
				commandExecutor: commandExecutor,
			}
		}

		// ✅ NEW: Easy to swap implementations for testing or different environments
		func (app *Application) ProcessAllMailboxes(ctx context.Context) error {
			mailboxes, err := app.mailboxRepo.GetMailboxes()
			if err != nil {
				return err
			}

			return app.mailboxService.ProcessMailboxes(ctx, mailboxes)
		}
	*/
}

// Benefits Summary:
//
// 1. **Better Testability**: Easy to create mocks and test in isolation
// 2. **Loose Coupling**: Depend on abstractions, not concrete implementations
// 3. **Extensibility**: Easy to add new mailbox types or behaviors
// 4. **Polymorphism**: Different implementations can be used interchangeably
// 5. **Dependency Injection**: Easy to swap implementations
// 6. **Maintainability**: Changes to implementations don't affect interface users
// 7. **Single Responsibility**: Each layer has a clear, focused purpose
//
// Migration Path:
//
// 1. ✅ Fixed interface definition to match implementation
// 2. ✅ Created interface-based factory functions
// 3. ✅ Added service layer for business logic
// 4. ✅ Implemented repository pattern for data access
// 5. ✅ Added command pattern for flexible operations
// 6. ✅ Created comprehensive mocks for testing
// 7. ✅ Updated existing code to use new patterns where beneficial
//
// The existing code continues to work, but new code can leverage these patterns
// for better architecture and maintainability.
