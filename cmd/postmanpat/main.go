package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"

	"aaronromeo.com/postmanpat/pkg/base"
	imap "aaronromeo.com/postmanpat/pkg/models/imapmanager"
	"aaronromeo.com/postmanpat/pkg/utils"
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
		imap.WithFileManager(utils.OSFileManager{}),
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

	type exportedMailbox struct {
		Name       string `json:"name"`
		Deletable  bool   `json:"deletable"`
		Exportable bool   `json:"exportable"`
		Lifespan   int    `json:"lifespan"`
	}
	exportedMailboxes := make(map[string]exportedMailbox, len(verifiedMailboxObjs))
	for mailboxName, mailbox := range verifiedMailboxObjs {
		exportedMailboxes[mailboxName] = exportedMailbox{
			Name:       mailbox.Name,
			Deletable:  mailbox.Deletable,
			Exportable: mailbox.Exportable,
			Lifespan:   mailbox.Lifespan,
		}
	}

	encodedMailboxes, err := json.MarshalIndent(exportedMailboxes, "", "  ")
	if err != nil {
		log.Fatalf("Converting mailbox names to JSON error %+v", err)
	}

	if err := os.WriteFile(base.MailboxListFile, encodedMailboxes, 0644); err != nil {
		log.Fatalf("Writing mailbox names file error %+v", err)
	}

	err = verifiedMailboxObjs[os.Getenv("IMAP_FOLDER")].ExportMessages()
	if err != nil {
		log.Fatalf("Exporting mailbox `%s` error", os.Getenv("IMAP_FOLDER"))
	}

	log.Println("Done!")
}
