package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
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

	otelfiber "github.com/gofiber/contrib/otelfiber/v2"
	fiber "github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html/v2"
	"github.com/urfave/cli/v2"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var (
	tracer     = otel.Tracer(base.OTEL_NAME)
	otelLogger = otelslog.NewLogger(base.OTEL_NAME)
	// meter      = otel.Meter(base.OTEL_NAME)
	// rollCnt    metric.Int64Counter
)

func main() {
	// Debug: Print that main function started
	log.Printf("Main function started")
	log.Printf("os.Args at start: %v (length: %d)", os.Args, len(os.Args))

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
			err := os.Setenv(key, os.Getenv(fmt.Sprintf("%s%s", TF_VAR_PREFIX, key)))
			if err != nil {
				log.Printf("Error unable to set the env var: %s %s", key, err)
			}
		}
	}

	for _, key := range []string{
		DIGITALOCEAN_BUCKET_ACCESS_KEY,
		DIGITALOCEAN_BUCKET_SECRET_KEY,
		IMAP_URL,
		IMAP_USER,
		IMAP_PASS,
		base.UPTRACE_DSN_ENV_VAR,
	} {
		if _, ok := os.LookupEnv(key); !ok {
			log.Fatalf("Environment variable %s is not set\n", key)
		} else {
			log.Printf("Environment variable %s is set\n", key)
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

	ctx := context.Background()

	// Set up OpenTelemetry.
	otelShutdown, err := utils.SetupOTelSDK(ctx)
	if err != nil {
		return
	}
	// Handle shutdown properly so nothing leaks.
	defer func() {
		log.Printf("Handling shutdown: %v", otelShutdown(context.Background()))
	}()

	_, span := tracer.Start(ctx, base.OTEL_NAME)
	defer span.End()

	if otelLogger == nil {
		log.Fatalf("Failed to create logger (in main)")
	} else {
		otelLogger.Info("Logger created")
		otelLogger.InfoContext(ctx, "Logger created (with context)")
	}

	isi, err := imap.NewImapManager(
		// Connect to server
		imap.WithTLSConfig(os.Getenv(IMAP_URL), nil),
		imap.WithAuth(os.Getenv(IMAP_USER), os.Getenv(IMAP_PASS)),
		imap.WithCtx(ctx),
		imap.WithLogger(otelLogger),
		imap.WithFileManager(utils.OSFileManager{}), // TODO: What is this used for?
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

	log.Printf("About to create CLI app")
	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:    "mailboxnames",
				Aliases: []string{"mn"},
				Usage:   "List mailbox names",
				Action:  listMailboxNames(ctx, isi, fileMgr),
			},
			{
				Name:    "reapmessages",
				Aliases: []string{"re"},
				Usage:   "Reap the messages in a mailbox",
				Action:  reapMessages(ctx, isi, fileMgr),
			},
			{
				Name:    "webserver",
				Aliases: []string{"ws"},
				Usage:   "Start the web server",
				Action:  webserver(ctx, fileMgr),
			},
		},
	}

	// Debug: Print os.Args to understand what's being passed
	log.Printf("os.Args: %v (length: %d)", os.Args, len(os.Args))

	// If no command is provided, default to webserver
	if len(os.Args) == 1 {
		log.Printf("No command provided, starting webserver")
		// Directly call the webserver function instead of modifying os.Args
		wsAction := webserver(ctx, fileMgr)
		if err := wsAction(nil); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func listMailboxNames(ctx context.Context, isi *imap.ImapManagerImpl, fileMgr utils.FileManager) func(c *cli.Context) error {
	return func(c *cli.Context) error {
		_, span := tracer.Start(ctx, "listMailboxNames")
		defer span.End()

		// List mailboxes
		verifiedMailboxObjs, err := isi.GetMailboxes()
		if err != nil {
			return errors.Errorf("getting mailboxes error %+v", err)
		}

		exportedMailboxes := make(map[string]base.SerializedMailbox, len(verifiedMailboxObjs))
		for mailboxName, mailbox := range verifiedMailboxObjs {
			exportedMailboxes[mailboxName] = base.SerializedMailbox{
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

		span.SetAttributes(
			attribute.String("mailboxListFile.name", base.MailboxListFile),
			attribute.Int("encodedMailboxes.count", len(encodedMailboxes)),
		)
		if err := fileMgr.WriteFile(base.MailboxListFile, encodedMailboxes, 0644); err != nil {
			return errors.Errorf("writing mailbox names file error %+v", err)
		}

		return nil
	}
}

func reapMessages(ctx context.Context, isi *imap.ImapManagerImpl, fileMgr utils.FileManager) func(c *cli.Context) error {
	return func(c *cli.Context) error {
		ctx, span := tracer.Start(ctx, "reapMessages")
		defer span.End()

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
			mailbox.Client = isi.Client
			mailbox.Logger = isi.Logger

			err := mailbox.ProcessMailbox(ctx)
			if err != nil {
				return errors.Errorf("unable to process mailboxes %+v", err)
			}
		}

		return nil
	}
}

func webserver(ctx context.Context, fileMgr utils.FileManager) func(c *cli.Context) error {
	return func(c *cli.Context) error {
		_, span := tracer.Start(ctx, "webserver")
		defer span.End()

		// Create view engine
		engine := html.New("./views", ".html")

		// Disable this in production
		engine.Reload(true)

		engine.AddFunc("getCssAsset", func(name string) (res template.HTML) {
			err := filepath.Walk("public/assets", func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.Name() == name {
					res = template.HTML("<link rel=\"stylesheet\" href=\"/" + path + "\">")
				}
				return nil
			})
			if err != nil {
				log.Printf("unable to process mailboxes %+v", err)
			}
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
		app.Use(otelfiber.Middleware())

		app.Use(func(c *fiber.Ctx) error {
			c.Locals("fileMgr", fileMgr)
			return c.Next()
		})

		// Setup routes
		app.Get("/", handlers.Home)
		app.Get("/about", handlers.About)
		app.Get("/mailboxes", handlers.Mailboxes)

		// Setup static files
		app.Static("/public", "./public")

		// Handle not founds
		app.Use(handlers.NotFound)

		// Start the server on port 3000
		return app.Listen(":3000")
	}
}
