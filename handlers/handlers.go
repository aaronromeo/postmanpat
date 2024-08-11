package handlers

import (
	"encoding/json"
	"fmt"

	"aaronromeo.com/postmanpat/pkg/base"
	"aaronromeo.com/postmanpat/pkg/models/mailbox"
	"aaronromeo.com/postmanpat/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/pkg/errors"
)

// Home renders the home view
func Home(c *fiber.Ctx) error {
	return c.Render("index", fiber.Map{
		"Title": "Hello, World!",
	})
}

// About renders the about view
func About(c *fiber.Ctx) error {
	return c.Render("about", nil)
}

// NoutFound renders the 404 view
func NotFound(c *fiber.Ctx) error {
	return c.Status(404).Render("404", nil)
}

// Home renders the home view
func Mailboxes(c *fiber.Ctx) error {
	fileMgr, ok := c.Locals("fileMgr").(utils.FileManager)
	if !ok {
		return c.Status(fiber.StatusInternalServerError).SendString("Could not retrieve file manager")
	}

	data, err := fileMgr.ReadFile(base.MailboxListFile)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(
			fmt.Sprintf("Reading mailbox error %+v", err),
		)
	}
	defer fileMgr.Close() //nolint:errcheck

	mailboxes := make(map[string]mailbox.MailboxImpl)

	err = json.Unmarshal(data, &mailboxes)
	if err != nil {
		return errors.Errorf("unable to marshal mailboxes %+v", err)
	}

	return c.Render("mailboxes/index", fiber.Map{
		"Title":     "Hello, World!",
		"Mailboxes": mailboxes,
	})
}
