// Package repositories provides data access layer for PostmanPat.
// This package implements the repository pattern to abstract data access
// and improve testability and maintainability.
package repositories

import (
	"encoding/json"
	"log/slog"

	"aaronromeo.com/postmanpat/pkg/base"
	"aaronromeo.com/postmanpat/pkg/models/imapmanager"
	"aaronromeo.com/postmanpat/pkg/models/mailbox"
	"aaronromeo.com/postmanpat/pkg/utils"
)

// MailboxRepository defines the interface for mailbox data operations.
type MailboxRepository interface {
	GetMailboxes() ([]mailbox.Mailbox, error)
	GetMailboxesAsMap() (map[string]mailbox.Mailbox, error)
	SaveMailboxes(mailboxes []mailbox.Mailbox) error
	LoadMailboxesFromFile() ([]mailbox.Mailbox, error)
}

// ImapMailboxRepository implements MailboxRepository using IMAP as the data source.
type ImapMailboxRepository struct {
	imapManager imapmanager.ImapManager
	fileManager utils.FileManager
	logger      *slog.Logger
}

// NewImapMailboxRepository creates a new IMAP-based mailbox repository.
func NewImapMailboxRepository(
	imapManager imapmanager.ImapManager,
	fileManager utils.FileManager,
	logger *slog.Logger,
) MailboxRepository {
	return &ImapMailboxRepository{
		imapManager: imapManager,
		fileManager: fileManager,
		logger:      logger,
	}
}

// GetMailboxes retrieves mailboxes from the IMAP server and returns them as interfaces.
func (r *ImapMailboxRepository) GetMailboxes() ([]mailbox.Mailbox, error) {
	impls, err := r.imapManager.GetMailboxes()
	if err != nil {
		r.logger.Error("Failed to get mailboxes from IMAP manager", 
			slog.String("error", err.Error()))
		return nil, err
	}

	// Convert map of implementations to slice of interfaces
	mailboxes := make([]mailbox.Mailbox, 0, len(impls))
	for _, impl := range impls {
		mailboxes = append(mailboxes, impl)
	}

	r.logger.Info("Successfully retrieved mailboxes", 
		slog.Int("count", len(mailboxes)))
	return mailboxes, nil
}

// GetMailboxesAsMap retrieves mailboxes as a map with names as keys.
func (r *ImapMailboxRepository) GetMailboxesAsMap() (map[string]mailbox.Mailbox, error) {
	impls, err := r.imapManager.GetMailboxes()
	if err != nil {
		r.logger.Error("Failed to get mailboxes from IMAP manager", 
			slog.String("error", err.Error()))
		return nil, err
	}

	// Convert map of implementations to map of interfaces
	mailboxes := make(map[string]mailbox.Mailbox, len(impls))
	for name, impl := range impls {
		mailboxes[name] = impl
	}

	r.logger.Info("Successfully retrieved mailboxes as map", 
		slog.Int("count", len(mailboxes)))
	return mailboxes, nil
}

// SaveMailboxes serializes and saves mailboxes to the file system.
func (r *ImapMailboxRepository) SaveMailboxes(mailboxes []mailbox.Mailbox) error {
	serializedMailboxes := make(map[string]base.SerializedMailbox, len(mailboxes))
	
	for _, mb := range mailboxes {
		serialized, err := mb.Serialize()
		if err != nil {
			r.logger.Error("Failed to serialize mailbox", 
				slog.String("error", err.Error()))
			return err
		}
		serializedMailboxes[serialized.Name] = serialized
	}

	data, err := json.MarshalIndent(serializedMailboxes, "", "  ")
	if err != nil {
		r.logger.Error("Failed to marshal mailboxes to JSON", 
			slog.String("error", err.Error()))
		return err
	}

	if err := r.fileManager.WriteFile(base.MailboxListFile, data, 0644); err != nil {
		r.logger.Error("Failed to write mailboxes to file", 
			slog.String("error", err.Error()))
		return err
	}

	r.logger.Info("Successfully saved mailboxes to file", 
		slog.Int("count", len(mailboxes)))
	return nil
}

// LoadMailboxesFromFile loads mailboxes from the serialized file.
func (r *ImapMailboxRepository) LoadMailboxesFromFile() ([]mailbox.Mailbox, error) {
	data, err := r.fileManager.ReadFile(base.MailboxListFile)
	if err != nil {
		r.logger.Error("Failed to read mailboxes file", 
			slog.String("error", err.Error()))
		return nil, err
	}

	var serializedMailboxes map[string]base.SerializedMailbox
	if err := json.Unmarshal(data, &serializedMailboxes); err != nil {
		r.logger.Error("Failed to unmarshal mailboxes from JSON", 
			slog.String("error", err.Error()))
		return nil, err
	}

	// Convert serialized mailboxes back to interface implementations
	// Note: This would need access to the IMAP manager to recreate full mailbox objects
	// For now, we'll use the UnserializeMailboxes method from the IMAP manager
	impls, err := r.imapManager.UnserializeMailboxes()
	if err != nil {
		r.logger.Error("Failed to unserialize mailboxes", 
			slog.String("error", err.Error()))
		return nil, err
	}

	mailboxes := make([]mailbox.Mailbox, 0, len(impls))
	for _, impl := range impls {
		mailboxes = append(mailboxes, impl)
	}

	r.logger.Info("Successfully loaded mailboxes from file", 
		slog.Int("count", len(mailboxes)))
	return mailboxes, nil
}
