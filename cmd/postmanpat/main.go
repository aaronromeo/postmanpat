package main

import (
	"encoding/json"
	"log"
	"os"

	"aaronromeo.com/postmanpat/internal/utils"
	"github.com/emersion/go-imap/client"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file: %s", err)
	}

	log.Println("Connecting to server...")

	// Connect to server
	c, err := client.DialTLS(os.Getenv("IMAP_URL"), nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Connected")

	// Don't forget to logout
	defer c.Logout()

	// Login
	if err := c.Login(os.Getenv("IMAP_USER"), os.Getenv("IMAP_PASS")); err != nil {
		log.Fatal(err)
	}
	log.Println("Logged in")

	// List mailboxes
	verifiedMailboxObjs := utils.ExportMailboxes(c)
	encodedMailboxes, err := json.MarshalIndent(verifiedMailboxObjs, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile(utils.MailboxListFile, encodedMailboxes, 0644); err != nil {
		log.Fatal(err)
	}

	utils.ExportEmails(c, os.Getenv("IMAP_FOLDER"))

	// // Define search criteria
	// criteria := imap.NewSearchCriteria()
	// criteria.Header.Add("From", "example@example.com")
	// criteria.Since = time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	log.Println("Done!")
}
