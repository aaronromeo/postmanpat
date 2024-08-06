package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"os"

	"aaronromeo.com/postmanpat/internal/handler/mailboxes"
	"aaronromeo.com/postmanpat/pkg/base"
	imap "aaronromeo.com/postmanpat/pkg/models/imapmanager"
	"aaronromeo.com/postmanpat/pkg/models/mailbox"
	"aaronromeo.com/postmanpat/pkg/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

const STORAGE_BUCKET = "postmanpat"

const DIGITALOCEAN_BUCKET_ACCESS_KEY = "DIGITALOCEAN_BUCKET_ACCESS_KEY"
const DIGITALOCEAN_BUCKET_SECRET_KEY = "DIGITALOCEAN_BUCKET_SECRET_KEY"
const IMAP_URL = "IMAP_URL"
const IMAP_USER = "IMAP_USER"
const IMAP_PASS = "IMAP_PASS"

func main() {
	// Connect to server
	// Check if the bucket exists
	// Create the bucket if it doesn't exist
	isi, fileMgr := envSetup()

	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:    "mailboxnames",
				Aliases: []string{"mn"},
				Usage:   "List mailbox names",
				Action:  listMailboxNames(isi, fileMgr),
			},
			{
				Name:    "reapmessages",
				Aliases: []string{"re"},
				Usage:   "Reap the messages in a mailbox",
				Action:  reapMessages(isi, fileMgr),
			},
			{
				Name:    "webserver",
				Aliases: []string{"ws"},
				Usage:   "Start the web server",
				Action:  webserver(),
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func envSetup() (*imap.ImapManagerImpl, *utils.S3FileManager) {
	err := godotenv.Load(".env")
	if err != nil {
		log.Printf("Error loading .env file, proceeding: %s", err)
	}

	for _, key := range []string{
		DIGITALOCEAN_BUCKET_ACCESS_KEY,
		DIGITALOCEAN_BUCKET_SECRET_KEY,
		IMAP_URL,
		IMAP_USER,
		IMAP_PASS,
	} {
		if _, ok := os.LookupEnv(key); !ok {
			log.Fatalf("Environment variable %s is not set", key)
		}
	}

	sess, err := session.NewSession(&aws.Config{
		Region:   aws.String("nyc3"),
		Endpoint: aws.String("nyc3.digitaloceanspaces.com"),
		Credentials: credentials.NewStaticCredentials(
			os.Getenv(DIGITALOCEAN_BUCKET_ACCESS_KEY),
			os.Getenv(DIGITALOCEAN_BUCKET_SECRET_KEY),
			"",
		),
	})
	if err != nil {
		log.Fatalf("Failed to create AWS session: %v", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()

	isi, err := imap.NewImapManager(

		imap.WithTLSConfig(os.Getenv(IMAP_URL), nil),
		imap.WithAuth(os.Getenv(IMAP_USER), os.Getenv(IMAP_PASS)),
		imap.WithCtx(ctx),
		imap.WithLogger(logger),
		imap.WithFileManager(utils.OSFileManager{}),
	)
	if err != nil {
		log.Fatal(err)
	}

	fileMgr := utils.NewS3FileManager(sess, STORAGE_BUCKET, isi.Username)

	exists, err := fileMgr.BucketExists(STORAGE_BUCKET)
	if err != nil {
		log.Fatalf("Failed to check if bucket exists: %v", err)
	}

	if exists {
		log.Printf("Found bucket %s\n", STORAGE_BUCKET)
	} else {

		err = fileMgr.CreateBucket(STORAGE_BUCKET)
		if err != nil {
			log.Fatalf("Failed to create bucket: %v", err)
		}
		log.Printf("Created the bucket %s\n", STORAGE_BUCKET)
	}
	return isi, fileMgr
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

func reapMessages(_ *imap.ImapManagerImpl, fileMgr utils.FileManager) func(c *cli.Context) error {
	return func(c *cli.Context) error {
		// Read the mailbox list file
		data, err := fileMgr.ReadFile(base.MailboxListFile)
		if err != nil {
			return errors.Errorf("exporting mailbox error %+v", err)
		}
		mailboxes := make(map[string]mailbox.MailboxImpl)

		err = json.Unmarshal(data, &mailboxes)
		if err != nil {
			return errors.Errorf("unable to marshal mailboxes %+v", err)
		}

		for _, mailbox := range mailboxes {
			err := mailbox.ProcessMailbox()
			if err != nil {
				return errors.Errorf("unable to process mailboxes %+v", err)
			}
		}

		return nil
	}
}

func webserver() func(c *cli.Context) error {
	return func(c *cli.Context) error {
		r := mux.NewRouter()
		r.HandleFunc("/", mailboxes.IndexPage).Methods("GET")
		http.Handle("/", r)

		log.Printf("Starting server on :8000")
		http.ListenAndServe(":8000", nil)
		// http.HandleFunc("/", handler.IndexPage)
		// http.ListenAndServe(":8000", nil)

		return nil
	}
}
