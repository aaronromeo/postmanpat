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
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

const STORAGE_BUCKET = "postmanpat"

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file: %s", err)
	}

	sess, err := session.NewSession(&aws.Config{
		Region:   aws.String("nyc3"),
		Endpoint: aws.String("nyc3.digitaloceanspaces.com"),
		Credentials: credentials.NewStaticCredentials(
			os.Getenv("DIGITALOCEAN_BUCKET_ACCESS_KEY"),
			os.Getenv("DIGITALOCEAN_BUCKET_SECRET_KEY"),
			"",
		),
	})
	if err != nil {
		log.Fatalf("Failed to create AWS session: %v", err)
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

	fileMgr := utils.NewS3FileManager(sess, STORAGE_BUCKET, isi.Username)

	// Check if the bucket exists
	exists, err := fileMgr.BucketExists(STORAGE_BUCKET)
	if err != nil {
		log.Fatalf("Failed to check if bucket exists: %v", err)
	}

	if exists {
		log.Printf("Found bucket %s\n", STORAGE_BUCKET)
	} else {
		// Create the bucket if it doesn't exist
		err = fileMgr.CreateBucket(STORAGE_BUCKET)
		if err != nil {
			log.Fatalf("Failed to create bucket: %v", err)
		}
		log.Printf("Created the bucket %s\n", STORAGE_BUCKET)
	}

	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:    "mailboxnames",
				Aliases: []string{"mn"},
				Usage:   "List mailbox names",
				Action:  listMailboxNames(isi, fileMgr),
			},
			{
				Name:    "exportmessages",
				Aliases: []string{"em"},
				Usage:   "Export the messages in a mailbox",
				Action:  exportMessages(isi, fileMgr),
				// Flags: []cli.Flag{
				// 	&cli.StringFlag{
				// 		Name:     "mailbox",
				// 		Usage:    "the name of the mailbox to export messages from",
				// 		Required: true,
				// 	},
				// },
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func listMailboxNames(isi *imap.ImapManagerImpl, fileMgr utils.FileManager) func(c *cli.Context) error {
	return func(c *cli.Context) error {
		// List mailboxes
		verifiedMailboxObjs, err := isi.GetMailboxes()
		if err != nil {
			return errors.Errorf("getting mailboxes error %+v", err)
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
			return errors.Errorf("converting mailbox names to JSON error %+v", err)
		}

		if err := fileMgr.WriteFile(base.MailboxListFile, encodedMailboxes, 0644); err != nil {
			return errors.Errorf("writing mailbox names file error %+v", err)
		}

		return nil
	}
}

func exportMessages(_ *imap.ImapManagerImpl, fileMgr utils.FileManager) func(c *cli.Context) error {
	return func(c *cli.Context) error {
		// mailboxName := c.String("mailbox")
		// err := isi   .ExportMessages()
		// if err != nil {
		// 	return errors.Errorf("exporting mailbox `%s` error", mailboxName)
		// }

		data, err := fileMgr.ReadFile(base.MailboxListFile)
		if err != nil {
			return errors.Errorf("exporting mailbox error", err)
		}
		log.Println(string(data))

		return nil
	}
}

// func main() {
// 	err := godotenv.Load(".env")
// 	if err != nil {
// 		log.Fatalf("Error loading .env file: %s", err)
// 	}

// 	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
// 	ctx := context.Background()

// 	isi, err := imap.NewImapManager(
// 		// Connect to server
// 		imap.WithTLSConfig(os.Getenv("IMAP_URL"), nil),
// 		imap.WithAuth(os.Getenv("IMAP_USER"), os.Getenv("IMAP_PASS")),
// 		imap.WithCtx(ctx),
// 		imap.WithLogger(logger),
// 		imap.WithFileManager(utils.OSFileManager{}),
// 	)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	log.Println("Connecting to server...")

// 	// List mailboxes
// 	verifiedMailboxObjs, err := isi.GetMailboxes()
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	type exportedMailbox struct {
// 		Name       string `json:"name"`
// 		Deletable  bool   `json:"deletable"`
// 		Exportable bool   `json:"exportable"`
// 		Lifespan   int    `json:"lifespan"`
// 	}
// 	exportedMailboxes := make(map[string]exportedMailbox, len(verifiedMailboxObjs))
// 	for mailboxName, mailbox := range verifiedMailboxObjs {
// 		exportedMailboxes[mailboxName] = exportedMailbox{
// 			Name:       mailbox.Name,
// 			Deletable:  mailbox.Deletable,
// 			Exportable: mailbox.Exportable,
// 			Lifespan:   mailbox.Lifespan,
// 		}
// 	}

// 	encodedMailboxes, err := json.MarshalIndent(exportedMailboxes, "", "  ")
// 	if err != nil {
// 		log.Fatalf("Converting mailbox names to JSON error %+v", err)
// 	}

// 	if err := os.WriteFile(base.MailboxListFile, encodedMailboxes, 0644); err != nil {
// 		log.Fatalf("Writing mailbox names file error %+v", err)
// 	}

// 	err = verifiedMailboxObjs[os.Getenv("IMAP_FOLDER")].ExportMessages()
// 	if err != nil {
// 		log.Fatalf("Exporting mailbox `%s` error", os.Getenv("IMAP_FOLDER"))
// 	}

// 	log.Println("Done!")
// }
