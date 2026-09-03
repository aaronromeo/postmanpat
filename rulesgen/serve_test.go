package rulesgen

import (
	"context"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

func waitHealthy(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not become healthy within 5s")
}

func getURL(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

func waitForBodyContains(t *testing.T, addr, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(getURL(t, "http://"+addr+"/"), want) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("body did not contain %q within 5s", want)
}

func TestServeIngestsAtStartupAndPollsForNewReports(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "postmanpat-analyze-analyze-inbox.json", reportA)
	addr := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, ServeOptions{
			Addr:       addr,
			ReportsDir: dir,
			DBPath:     filepath.Join(t.TempDir(), "queue.db"),
			PollEvery:  50 * time.Millisecond,
		})
	}()

	waitHealthy(t, addr)
	waitForBodyContains(t, addr, "sender-news")

	writeReport(t, dir, "postmanpat-analyze-analyze-inbox.json", reportB)
	waitForBodyContains(t, addr, "sender-newcomer")

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after context cancel")
	}
}
