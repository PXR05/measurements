package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"measurements/internal/ingest"
)

func main() {
	var o ingest.Options
	flag.StringVar(
		&o.DataDir, "data-dir", "internal/data", "directory for raw_cache and measurements.sqlite")
	flag.StringVar(&o.RegistryURL, "registry", ingest.RegistryURL, "registry URL")
	flag.StringVar(&o.Site, "site", "", "only this registry username (all databases)")
	flag.StringVar(&o.Phone, "phone", "", "only fetch measurements for matching brand/phone names")
	flag.IntVar(&o.Workers, "workers", 4, "parallel measurement downloads")
	flag.DurationVar(&o.Timeout, "timeout", 30*time.Second, "HTTP request timeout")
	flag.BoolVar(&o.Refresh, "refresh", false, "fetch fresh responses; retain previous cache objects and DB snapshots")
	flag.BoolVar(&o.Offline, "offline", false, "normalize using only the local raw cache")
	flag.Parse()

	if o.Workers < 1 || o.Workers > 64 {
		log.Fatal("workers must be between 1 and 64")
	}
	if o.Timeout <= 0 {
		log.Fatal("timeout must be positive")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	stats, err := ingest.Seed(ctx, o)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("seed complete: %d sources, %d selected phones, %d newly imported measurements, %d issues", stats.Sources, stats.Phones, stats.Measurements, stats.Failures)
}
