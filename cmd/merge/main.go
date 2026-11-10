package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"measurements/internal/identity"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	address := flag.String("addr", "127.0.0.1:8081", "loopback HTTP listen address")
	path := flag.String("db", "internal/data/measurements.sqlite", "existing seed database")
	restore := flag.String("import", "", "restore exported JSON into an empty review log, then exit")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	s, err := identity.Open(ctx, *path)
	if err != nil {
		return err
	}
	defer s.DB.Close()
	if *restore != "" {
		f, err := os.Open(*restore)
		if err != nil {
			return err
		}
		defer f.Close()
		var b identity.Backup
		if err = json.NewDecoder(f).Decode(&b); err != nil {
			return err
		}
		if err = s.Restore(ctx, b); err != nil {
			return err
		}
		log.Printf("restored %d identity decisions", len(b.Decisions))
		return nil
	}
	host, _, err := net.SplitHostPort(*address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("merge UI must listen on a loopback IP, for example 127.0.0.1:8081")
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return err
	}
	defer listener.Close()
	count, err := s.AutoMerge(ctx, filepath.Join(filepath.Dir(*path), "merge-backups"))
	if err != nil {
		return err
	}
	log.Printf("automerged %d IEM identities", count)
	server := &http.Server{Addr: *address, Handler: identity.NewHandler(s), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 2 * time.Minute, IdleTimeout: 60 * time.Second}
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			server.Close()
		}
	}()
	log.Printf("Catalog identity review: http://%s", *address)
	err = server.Serve(listener)
	stop()
	<-done
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
