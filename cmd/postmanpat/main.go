package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"

	"aaronromeo.com/postmanpat/internal/utils"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file: %s", err)
	}

	// Connect to server
	// c, err := client.DialTLS(os.Getenv("IMAP_URL"), nil)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// log.Println("Connected")

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()

	isi, err := utils.NewImapService(
		// Connect to server
		utils.WithTLSConfig(os.Getenv("IMAP_URL"), nil),
		utils.WithAuth(os.Getenv("IMAP_USER"), os.Getenv("IMAP_PASS")),
		utils.WithCtx(ctx),
		utils.WithLogger(logger),
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

	if err := os.WriteFile(utils.MailboxListFile, encodedMailboxes, 0644); err != nil {
		log.Fatal(err)
	}

	// utils.ExportEmailsFromMailbox(c, os.Getenv("IMAP_FOLDER"))

	log.Println("Done!")
}
