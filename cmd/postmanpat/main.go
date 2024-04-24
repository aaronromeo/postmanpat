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
	verifiedMailboxObjs := utils.GetMailboxes(c)
	encodedMailboxes, err := json.MarshalIndent(verifiedMailboxObjs, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile(utils.MailboxListFile, encodedMailboxes, 0644); err != nil {
		log.Fatal(err)
	}

	utils.ExportEmailsFromMailbox(c, os.Getenv("IMAP_FOLDER"))

	log.Println("Done!")
}
