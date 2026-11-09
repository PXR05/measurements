package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"measurements/internal/api"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	address := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")

	path := flag.String("db", "internal/data/measurements.sqlite", "existing seed database")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	db, err := api.OpenDatabase(ctx, *path)
	if err != nil {
		return err
	}
	defer db.Close()

	server := &http.Server{
		Addr:              *address,
		Handler:           api.New(db, log.Default()),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
			_ = server.Close()
		}
	}()

	log.Printf("serving %s at http://%s/api/v1/", *path, *address)

	err = server.ListenAndServe()
	stop()
	<-shutdownDone

	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}
