package mock

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"testing"
)

// setupLogger sets up a logger that only outputs if the test fails
func SetupLogger(t *testing.T) *slog.Logger {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	t.Cleanup(func() {
		if t.Failed() {
			os.Stdout.Write(buf.Bytes()) //nolint:errcheck
		}
	})

	return logger
}

// StringLiteral is a simple imap.Literal implementation that wraps a string.
type StringLiteral struct {
	s   string
	pos int
}

// NewStringLiteral creates a new StringLiteral based on a string.
func NewStringLiteral(s string) *StringLiteral {
	return &StringLiteral{s: s}
}

func (l *StringLiteral) Read(p []byte) (n int, err error) {
	if l.pos >= len(l.s) {
		return 0, io.EOF // If all bytes have been read, return EOF
	}

	// Copy bytes from the string to p
	n = copy(p, l.s[l.pos:])
	l.pos += n // Move the read position forward

	return n, nil
}

// Len returns the length of the underlying string.
func (l *StringLiteral) Len() int {
	return len(l.s)
}
