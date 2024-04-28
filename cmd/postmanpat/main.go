package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"

	"aaronromeo.com/postmanpat/pkg/base"
	"aaronromeo.com/postmanpat/pkg/models"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file: %s", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()

	isi, err := models.NewImapManager(
		// Connect to server
		models.WithTLSConfig(os.Getenv("IMAP_URL"), nil),
		models.WithAuth(os.Getenv("IMAP_USER"), os.Getenv("IMAP_PASS")),
		models.WithCtx(ctx),
		models.WithLogger(logger),
	)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Connecting to server...")

	// List mailboxes
	verifiedMailboxObjs, err := isi.GetMailboxes()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("After List mailboxes")

	encodedMailboxes, err := json.MarshalIndent(verifiedMailboxObjs, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile(base.MailboxListFile, encodedMailboxes, 0644); err != nil {
		log.Fatal(err)
	}

	// utils.ExportEmailsFromMailbox(c, os.Getenv("IMAP_FOLDER"))

	log.Println("Done!")
}
