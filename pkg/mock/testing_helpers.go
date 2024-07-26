package mock

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	imap "github.com/emersion/go-imap"
	gomock "go.uber.org/mock/gomock"
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

// Custom matcher to check if the Before field is within the tolerance
type searchCriteriaMatcher struct {
	criteria  *imap.SearchCriteria
	tolerance time.Duration
}

func (m searchCriteriaMatcher) Matches(x interface{}) bool {
	c, ok := x.(*imap.SearchCriteria)
	if !ok {
		return false
	}
	beforeDiff := c.Before.Sub(m.criteria.Before)
	return beforeDiff <= m.tolerance && beforeDiff >= -m.tolerance
}

func (m searchCriteriaMatcher) String() string {
	return "matches criteria within tolerance"
}

// NewSearchCriteriaMatcher returns a matcher for search criteria with a tolerance
func NewSearchCriteriaMatcher(criteria *imap.SearchCriteria, tolerance time.Duration) gomock.Matcher {
	return searchCriteriaMatcher{criteria: criteria, tolerance: tolerance}
}
