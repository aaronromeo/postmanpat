package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"

	"aaronromeo.com/postmanpat/pkg/base"
	imap "aaronromeo.com/postmanpat/pkg/models/imapmanager"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file: %s", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()

	isi, err := imap.NewImapManager(
		// Connect to server
		imap.WithTLSConfig(os.Getenv("IMAP_URL"), nil),
		imap.WithAuth(os.Getenv("IMAP_USER"), os.Getenv("IMAP_PASS")),
		imap.WithCtx(ctx),
		imap.WithLogger(logger),
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
	// for _, mailbox := range verifiedMailboxObjs {
	// log.Printf("Exporting messages from %s\n", mailbox.Name)
	err = verifiedMailboxObjs[os.Getenv("IMAP_FOLDER")].ExportMessages()
	if err != nil {
		log.Fatal(err)
	}
	// }

	log.Println("Done!")
}
