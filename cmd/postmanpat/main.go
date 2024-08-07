package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"aaronromeo.com/postmanpat/handlers"
	"aaronromeo.com/postmanpat/pkg/base"
	imap "aaronromeo.com/postmanpat/pkg/models/imapmanager"
	"aaronromeo.com/postmanpat/pkg/models/mailbox"
	"aaronromeo.com/postmanpat/pkg/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"

	// "github.com/gofiber/fiber/v3"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html/v2"
	"github.com/urfave/cli/v2"
)

const STORAGE_BUCKET = "postmanpat"

const TF_VAR_PREFIX = "TF_VAR_"
const DIGITALOCEAN_BUCKET_ACCESS_KEY = "DIGITALOCEAN_BUCKET_ACCESS_KEY"
const DIGITALOCEAN_BUCKET_SECRET_KEY = "DIGITALOCEAN_BUCKET_SECRET_KEY"
const IMAP_URL = "IMAP_URL"
const IMAP_USER = "IMAP_USER"
const IMAP_PASS = "IMAP_PASS"

func main() {
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
			os.Setenv(key, os.Getenv(fmt.Sprintf("%s%s", TF_VAR_PREFIX, key)))
		}
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
		// Connect to server
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
		// Create view engine
		engine := html.New("./views", ".html")

		// Disable this in production
		engine.Reload(true)

		engine.AddFunc("getCssAsset", func(name string) (res template.HTML) {
			filepath.Walk("public/assets", func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.Name() == name {
					res = template.HTML("<link rel=\"stylesheet\" href=\"/" + path + "\">")
				}
				return nil
			})
			return
		})

		// Create fiber app
		app := fiber.New(fiber.Config{
			Views:       engine,
			ViewsLayout: "layouts/main",
		})

		// Middleware
		app.Use(recover.New())
		app.Use(logger.New())

		// Setup routes
		app.Get("/", handlers.Home)
		app.Get("/about", handlers.About)

		// Setup static files
		app.Static("/public", "./public")

		// Handle not founds
		app.Use(handlers.NotFound)

		// Start the server on port 3000
		return app.Listen(":3000")
	}
}
