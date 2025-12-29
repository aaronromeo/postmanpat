PostmanPat is a Go-based email processing and archival system that connects to IMAP email servers to automatically manage email messages. It provides automated email archival, cleanup and filtering.

The consumer of the `postmanpat` application is myself. I use it to manage my personal email accounts and archive emails to DigitalOcean Spaces.

The service has two main components:
- A scheduled worker which processes emails in an IMAP account and moves them to the archive if required
- A on-demand service which is processes received emails and moves them to the required location

The application is deployed to DigitalOcean as a Docker container.
