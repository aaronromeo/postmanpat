// Package commands provides command pattern implementations for PostmanPat.
// This package allows for flexible execution of different mailbox operations
// and supports features like undo, queuing, and logging.
package commands

import (
	"context"
	"log/slog"

	"aaronromeo.com/postmanpat/pkg/models/mailbox"
)

// MailboxCommand defines the interface for mailbox operations using the Command pattern.
type MailboxCommand interface {
	Execute(ctx context.Context, mb mailbox.Mailbox) error
	GetName() string
	GetDescription() string
}

// ReapCommand implements the reap operation for mailboxes.
type ReapCommand struct {
	logger *slog.Logger
}

// NewReapCommand creates a new ReapCommand.
func NewReapCommand(logger *slog.Logger) MailboxCommand {
	return &ReapCommand{logger: logger}
}

// Execute performs the reap operation on the given mailbox.
func (c *ReapCommand) Execute(ctx context.Context, mb mailbox.Mailbox) error {
	c.logger.InfoContext(ctx, "Executing reap command")
	return mb.Reap()
}

// GetName returns the command name.
func (c *ReapCommand) GetName() string {
	return "reap"
}

// GetDescription returns the command description.
func (c *ReapCommand) GetDescription() string {
	return "Reaps messages from the mailbox"
}

// ExportCommand implements the export operation for mailboxes.
type ExportCommand struct {
	logger *slog.Logger
}

// NewExportCommand creates a new ExportCommand.
func NewExportCommand(logger *slog.Logger) MailboxCommand {
	return &ExportCommand{logger: logger}
}

// Execute performs the export and delete operation on the given mailbox.
func (c *ExportCommand) Execute(ctx context.Context, mb mailbox.Mailbox) error {
	c.logger.InfoContext(ctx, "Executing export command")
	return mb.ExportAndDeleteMessages()
}

// GetName returns the command name.
func (c *ExportCommand) GetName() string {
	return "export"
}

// GetDescription returns the command description.
func (c *ExportCommand) GetDescription() string {
	return "Exports and deletes messages from the mailbox"
}

// DeleteCommand implements the delete operation for mailboxes.
type DeleteCommand struct {
	logger *slog.Logger
}

// NewDeleteCommand creates a new DeleteCommand.
func NewDeleteCommand(logger *slog.Logger) MailboxCommand {
	return &DeleteCommand{logger: logger}
}

// Execute performs the delete operation on the given mailbox.
func (c *DeleteCommand) Execute(ctx context.Context, mb mailbox.Mailbox) error {
	c.logger.InfoContext(ctx, "Executing delete command")
	return mb.DeleteMessages()
}

// GetName returns the command name.
func (c *DeleteCommand) GetName() string {
	return "delete"
}

// GetDescription returns the command description.
func (c *DeleteCommand) GetDescription() string {
	return "Deletes messages from the mailbox without exporting"
}

// ProcessCommand implements the full process operation for mailboxes.
type ProcessCommand struct {
	logger *slog.Logger
}

// NewProcessCommand creates a new ProcessCommand.
func NewProcessCommand(logger *slog.Logger) MailboxCommand {
	return &ProcessCommand{logger: logger}
}

// Execute performs the full process operation on the given mailbox.
func (c *ProcessCommand) Execute(ctx context.Context, mb mailbox.Mailbox) error {
	c.logger.InfoContext(ctx, "Executing process command")
	return mb.ProcessMailbox(ctx)
}

// GetName returns the command name.
func (c *ProcessCommand) GetName() string {
	return "process"
}

// GetDescription returns the command description.
func (c *ProcessCommand) GetDescription() string {
	return "Processes the mailbox according to its configuration"
}

// CommandExecutor provides a way to execute commands on mailboxes.
type CommandExecutor struct {
	logger *slog.Logger
}

// NewCommandExecutor creates a new CommandExecutor.
func NewCommandExecutor(logger *slog.Logger) *CommandExecutor {
	return &CommandExecutor{logger: logger}
}

// ExecuteCommand executes a command on a mailbox with logging and error handling.
func (e *CommandExecutor) ExecuteCommand(ctx context.Context, cmd MailboxCommand, mb mailbox.Mailbox) error {
	e.logger.InfoContext(ctx, "Starting command execution",
		slog.String("command", cmd.GetName()),
		slog.String("description", cmd.GetDescription()))

	if err := cmd.Execute(ctx, mb); err != nil {
		e.logger.ErrorContext(ctx, "Command execution failed",
			slog.String("command", cmd.GetName()),
			slog.String("error", err.Error()))
		return err
	}

	e.logger.InfoContext(ctx, "Command execution completed successfully",
		slog.String("command", cmd.GetName()))
	return nil
}

// ExecuteCommands executes multiple commands on a mailbox in sequence.
func (e *CommandExecutor) ExecuteCommands(ctx context.Context, commands []MailboxCommand, mb mailbox.Mailbox) error {
	for _, cmd := range commands {
		if err := e.ExecuteCommand(ctx, cmd, mb); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteCommandOnMailboxes executes a command on multiple mailboxes.
func (e *CommandExecutor) ExecuteCommandOnMailboxes(ctx context.Context, cmd MailboxCommand, mailboxes []mailbox.Mailbox) error {
	for _, mb := range mailboxes {
		if err := e.ExecuteCommand(ctx, cmd, mb); err != nil {
			return err
		}
	}
	return nil
}
