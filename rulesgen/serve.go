package rulesgen

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"
)

type ServeOptions struct {
	Addr       string
	ReportsDir string
	DBPath     string
	PollEvery  time.Duration
}

func Serve(ctx context.Context, opts ServeOptions) error {
	if opts.PollEvery <= 0 {
		opts.PollEvery = time.Minute
	}
	st, err := Open(opts.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	if err := IngestDir(opts.ReportsDir, st); err != nil {
		return err
	}

	ticker := time.NewTicker(opts.PollEvery)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := IngestDir(opts.ReportsDir, st); err != nil {
					log.Printf("rulesgen serve: re-ingest: %v", err)
				}
			}
		}
	}()

	srv := &http.Server{Addr: opts.Addr, Handler: NewServer(st)}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("rulesgen serve: shutdown: %v", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
