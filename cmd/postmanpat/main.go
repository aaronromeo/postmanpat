package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aaronromeo/postmanpat/cli"
	"github.com/aaronromeo/postmanpat/obs"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdown, err := obs.Init(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "observability init failed:", err)
		os.Exit(1)
	}

	code := cli.ExecuteWithContext(ctx)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdown(shutdownCtx); err != nil {
		fmt.Fprintln(os.Stderr, "observability shutdown failed:", err)
		code = 1
	}
	os.Exit(code)
}
