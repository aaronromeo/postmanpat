// Package services provides business logic services for PostmanPat.
// This package implements the service layer pattern to separate business logic
// from infrastructure concerns and improve testability.
package services

import (
	"context"
	"log/slog"

	"aaronromeo.com/postmanpat/pkg/models/mailbox"
)

// MailboxService defines the interface for mailbox business operations.
// This interface allows for easy testing and different implementations.
type MailboxService interface {
	ProcessMailboxes(ctx context.Context, mailboxes []mailbox.Mailbox) error
	ReapMailbox(ctx context.Context, mb mailbox.Mailbox) error
	ExportMailbox(ctx context.Context, mb mailbox.Mailbox) error
	DeleteMailbox(ctx context.Context, mb mailbox.Mailbox) error
}

// MailboxServiceImpl implements the MailboxService interface.
type MailboxServiceImpl struct {
	logger *slog.Logger
}

// NewMailboxService creates a new MailboxService implementation.
func NewMailboxService(logger *slog.Logger) MailboxService {
	return &MailboxServiceImpl{
		logger: logger,
	}
}

// ProcessMailboxes processes a collection of mailboxes based on their configuration.
// This method implements the core business logic for mailbox processing.
func (s *MailboxServiceImpl) ProcessMailboxes(ctx context.Context, mailboxes []mailbox.Mailbox) error {
	for _, mb := range mailboxes {
		if err := mb.ProcessMailbox(ctx); err != nil {
			s.logger.ErrorContext(ctx, "Failed to process mailbox", 
				slog.String("error", err.Error()))
			return err
		}
	}
	return nil
}

// ReapMailbox performs the reap operation on a single mailbox.
func (s *MailboxServiceImpl) ReapMailbox(ctx context.Context, mb mailbox.Mailbox) error {
	s.logger.InfoContext(ctx, "Starting mailbox reap operation")
	
	if err := mb.Reap(); err != nil {
		s.logger.ErrorContext(ctx, "Failed to reap mailbox", 
			slog.String("error", err.Error()))
		return err
	}
	
	s.logger.InfoContext(ctx, "Mailbox reap operation completed successfully")
	return nil
}

// ExportMailbox exports and optionally deletes messages from a mailbox.
func (s *MailboxServiceImpl) ExportMailbox(ctx context.Context, mb mailbox.Mailbox) error {
	s.logger.InfoContext(ctx, "Starting mailbox export operation")
	
	if err := mb.ExportAndDeleteMessages(); err != nil {
		s.logger.ErrorContext(ctx, "Failed to export mailbox", 
			slog.String("error", err.Error()))
		return err
	}
	
	s.logger.InfoContext(ctx, "Mailbox export operation completed successfully")
	return nil
}

// DeleteMailbox deletes messages from a mailbox without exporting.
func (s *MailboxServiceImpl) DeleteMailbox(ctx context.Context, mb mailbox.Mailbox) error {
	s.logger.InfoContext(ctx, "Starting mailbox delete operation")
	
	if err := mb.DeleteMessages(); err != nil {
		s.logger.ErrorContext(ctx, "Failed to delete mailbox messages", 
			slog.String("error", err.Error()))
		return err
	}
	
	s.logger.InfoContext(ctx, "Mailbox delete operation completed successfully")
	return nil
}
