package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"measurements/internal/storage"
	"measurements/internal/storage/dbgen"
)

const RegistryURL = "https://squig.link/squigsites.json"

type Options struct {
	RegistryURL, DataDir string
	Site, Phone          string
	Workers              int
	Timeout              time.Duration
	Offline, Refresh     bool
	Client               *http.Client
	Logger               *log.Logger
}
type Stats struct{ Sources, Phones, Measurements, Failures int }
type seeder struct {
	db    *sql.DB
	cache cache
	opts  Options
	run   int64
	stats Stats
}
type measurementJob struct {
	variant              int64
	url, channel, sample string
}
type measurementResult struct {
	job      measurementJob
	resource resource
	err      error
}

func Seed(ctx context.Context, o Options) (stats Stats, retErr error) {
	if o.RegistryURL == "" {
		o.RegistryURL = RegistryURL
	}
	if o.DataDir == "" {
		o.DataDir = filepath.Join("internal", "data")
	}
	if o.Workers <= 0 {
		o.Workers = 4
	}
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
	if o.Offline && o.Refresh {
		return stats, errors.New("offline and refresh cannot be combined")
	}
	if o.Client == nil {
		o.Client = &http.Client{Timeout: o.Timeout}
	}
	if o.Logger == nil {
		o.Logger = log.Default()
	}

	db, err := openStore(filepath.Join(o.DataDir, "measurements.sqlite"))
	if err != nil {
		return stats, err
	}
	defer db.Close()

	s := seeder{
		db:   db,
		opts: o,
		cache: cache{
			root:    filepath.Join(o.DataDir, "raw_cache"),
			client:  o.Client,
			offline: o.Offline,
			refresh: o.Refresh,
		},
	}

	s.run, err = dbgen.New(db).CreateRun(ctx, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return stats, err
	}

	defer func() {
		status := "complete"
		if retErr != nil {
			status = "failed"
		} else if s.stats.Failures > 0 {
			status = "complete_with_issues"
		}
		err := dbgen.New(db).FinishRun(context.Background(), dbgen.FinishRunParams{
			FinishedAt: sql.NullString{
				String: time.Now().UTC().Format(time.RFC3339),
				Valid:  true,
			},
			Status: status,
			ID:     s.run,
		})
		if err != nil {
			retErr = errors.Join(retErr, err)
		}
		stats = s.stats
	}()

	r, err := s.cache.fetch(ctx, o.RegistryURL)
	if err != nil {
		return stats, err
	}

	rid, err := saveResource(ctx, db, r)
	if err != nil {
		return stats, err
	}

	if r.Status != http.StatusOK {
		return stats, fmt.Errorf("registry: HTTP %d", r.Status)
	}

	var sites []json.RawMessage
	if err = json.Unmarshal(r.Body, &sites); err != nil {
		return stats, fmt.Errorf("registry: %w", err)
	}

	matched := false
	for i, raw := range sites {
		if err = ctx.Err(); err != nil {
			return stats, err
		}

		var site SquigSite
		if err = json.Unmarshal(raw, &site); err != nil {
			if err = s.issue(ctx, r.URL, fmt.Errorf("site %d: %w", i, err)); err != nil {
				return stats, err
			}
			continue
		}

		if o.Site != "" && !strings.EqualFold(site.Username, o.Site) {
			continue
		}
		matched = true

		sid, err := dbgen.New(db).UpsertSite(
			ctx,
			dbgen.UpsertSiteParams{
				RegistryResourceID: rid,
				Ordinal:            int64(i),
				Username:           site.Username,
				Name:               site.Name,
				URLType:            site.URLType,
				AltDomain:          site.AltDomain,
				RawJSON:            string(raw),
			},
		)
		if err != nil {
			return stats, err
		}

		var siteRaw struct {
			DBs []json.RawMessage `json:"dbs"`
		}
		if err = json.Unmarshal(raw, &siteRaw); err != nil {
			return stats, err
		}

		for j, d := range site.DBs {
			address, err := DataURL(site, d)
			if err != nil {
				if err = s.issue(ctx, r.URL, err); err != nil {
					return stats, err
				}
				continue
			}

			source, err := dbgen.New(db).UpsertSource(
				ctx,
				dbgen.UpsertSourceParams{
					SiteID:     sid,
					Ordinal:    int64(j),
					Type:       d.Type,
					Folder:     d.Folder,
					DeltaReady: boolInt(bool(d.DeltaReady)),
					DataURL:    address,
					RawJSON:    string(siteRaw.DBs[j]),
				},
			)
			if err != nil {
				return stats, err
			}

			o.Logger.Printf("seeding %s (%s): %s", site.Username, d.Type, address)
			if err = s.source(ctx, source, address); err != nil {
				return stats, err
			}
		}
	}

	if !matched && o.Site != "" {
		return stats, fmt.Errorf("site %q not found", o.Site)
	}

	if err = storage.InstallViews(ctx, db); err != nil {
		return stats, err
	}

	return stats, nil
}

func (s *seeder) issue(ctx context.Context, address string, err error) error {
	var local cacheError
	if errors.As(err, &local) {
		return fmt.Errorf("local cache failure: %w", err)
	}
	s.stats.Failures++
	s.opts.Logger.Printf("skip %s: %v", address, err)
	e := dbgen.New(s.db).CreateIssue(
		ctx,
		dbgen.CreateIssueParams{
			RunID:   s.run,
			URL:     address,
			Message: err.Error(),
		},
	)
	return e
}

func (s *seeder) source(ctx context.Context, source int64, address string) error {
	bookURL := fileURL(address, "phone_book.json")

	r, err := s.cache.fetch(ctx, bookURL)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return s.issue(ctx, bookURL, err)
	}

	rid, err := saveResource(ctx, s.db, r)
	if err != nil {
		return err
	}
	if r.Status != 200 {
		return s.issue(ctx, bookURL, fmt.Errorf("HTTP %d", r.Status))
	}

	var brands []json.RawMessage
	if err = json.Unmarshal(r.Body, &brands); err != nil {
		return s.issue(ctx, bookURL, err)
	}

	base, _ := url.Parse(address)
	base.Path = strings.TrimSuffix(base.Path, "data/")
	configURL := fileURL(base.String(), "config.js")
	cfg, err := s.cache.fetch(ctx, configURL)
	l := layout{Directory: "data/", Channels: []string{"L", "R"}}
	var configID *int64
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err = s.issue(ctx, configURL, err); err != nil {
			return err
		}
	} else {
		configResourceID, saveErr := saveResource(ctx, s.db, cfg)
		configID, err = &configResourceID, saveErr
		if err != nil {
			return err
		}

		if cfg.Status == 200 {
			l, err = parseLayout(cfg.Body)
			if err != nil {
				return s.issue(ctx, configURL, err)
			}
		} else {
			if err = s.issue(ctx, configURL, fmt.Errorf("HTTP %d; using default layout", cfg.Status)); err != nil {
				return err
			}
		}
	}

	directory, err := url.Parse(l.Directory)
	if err != nil {
		return s.issue(ctx, configURL, err)
	}

	dataURL := base.ResolveReference(directory).String()
	if !strings.HasSuffix(dataURL, "/") {
		dataURL += "/"
	}

	book, err := dbgen.New(s.db).UpsertPhoneBook(
		ctx,
		dbgen.UpsertPhoneBookParams{
			SourceID:         source,
			ResourceID:       rid,
			ConfigResourceID: nullableID(configID),
		},
	)
	if err != nil {
		return err
	}

	s.stats.Sources++

	var jobs []measurementJob
	for bi, raw := range brands {
		var brand struct {
			Name   string            `json:"name"`
			Phones []json.RawMessage `json:"phones"`
		}
		if err = json.Unmarshal(raw, &brand); err != nil {
			if err = s.issue(ctx, bookURL, err); err != nil {
				return err
			}
			continue
		}

		bid, err := dbgen.New(s.db).UpsertBrand(
			ctx,
			dbgen.UpsertBrandParams{
				PhoneBookID: book,
				Ordinal:     int64(bi),
				Name:        brand.Name,
				RawJSON:     string(raw),
			})
		if err != nil {
			return err
		}

		for pi, phoneRaw := range brand.Phones {
			var fields map[string]json.RawMessage
			if err = json.Unmarshal(phoneRaw, &fields); err != nil {
				if err = s.issue(ctx, bookURL, err); err != nil {
					return err
				}
				continue
			}

			scalar := func(key string) string {
				v := fields[key]
				var str string
				if json.Unmarshal(v, &str) == nil {
					return str
				}
				if string(v) == "null" {
					return ""
				}
				return string(v)
			}

			pid, err := dbgen.New(s.db).UpsertPhone(
				ctx,
				dbgen.UpsertPhoneParams{
					BrandID:     bid,
					Ordinal:     int64(pi),
					Name:        scalar("name"),
					ReviewScore: scalar("reviewScore"),
					ReviewLink:  scalar("reviewLink"),
					Price:       scalar("price"),
					ShopLink:    scalar("shopLink"),
					RawJSON:     string(phoneRaw),
				},
			)
			if err != nil {
				return err
			}

			var files, suffixes StringSlice
			if v, ok := fields["file"]; ok {
				if err = json.Unmarshal(v, &files); err != nil {
					if err = s.issue(ctx, bookURL, err); err != nil {
						return err
					}
					continue
				}
			}

			if v, ok := fields["suffix"]; ok {
				if err = json.Unmarshal(v, &suffixes); err != nil {
					if err = s.issue(ctx, bookURL, err); err != nil {
						return err
					}
					continue
				}
			}

			selected := s.opts.Phone == "" || strings.Contains(strings.ToLower(brand.Name+" "+scalar("name")), strings.ToLower(s.opts.Phone))
			if selected {
				s.stats.Phones++
			}

			for fi, stem := range files {
				suffix := ""
				if fi < len(suffixes) {
					suffix = suffixes[fi]
				}

				variant, err := dbgen.New(s.db).UpsertVariant(
					ctx,
					dbgen.UpsertVariantParams{
						PhoneID:  pid,
						Ordinal:  int64(fi),
						FileStem: stem,
						Suffix:   suffix,
					})
				if err != nil {
					return err
				}
				if !selected {
					continue
				}

				for _, ch := range l.Channels {
					if l.Samples == 0 {
						jobs = append(
							jobs,
							measurementJob{
								variant: variant,
								url:     fileURL(dataURL, stem+" "+ch+".txt"),
								channel: ch,
								sample:  "",
							},
						)
					} else {
						for n := 1; n <= l.Samples; n++ {
							sample := fmt.Sprint(n)
							jobs = append(
								jobs,
								measurementJob{
									variant: variant,
									url:     fileURL(dataURL, stem+" "+ch+sample+".txt"),
									channel: ch,
									sample:  sample,
								},
							)
						}
					}
				}
			}
		}
	}

	return s.measurements(ctx, jobs)
}

func (s *seeder) measurements(ctx context.Context, jobs []measurementJob) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	work := make(chan measurementJob)
	results := make(chan measurementResult, s.opts.Workers)

	var wg sync.WaitGroup
	for i := 0; i < s.opts.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range work {
				r, err := s.cache.fetch(ctx, j.url)
				select {
				case results <- measurementResult{j, r, err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		defer close(work)
		for _, j := range jobs {
			select {
			case work <- j:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() { wg.Wait(); close(results) }()

	defer func() {
		cancel()
		for range results {
		}
	}()

	for result := range results {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if result.err != nil {
			if err := s.issue(ctx, result.job.url, result.err); err != nil {
				return err
			}
			continue
		}

		rid, err := saveResource(ctx, s.db, result.resource)
		if err != nil {
			return err
		}
		if result.resource.Status == http.StatusNotFound {
			continue
		}

		good, err := saveMeasurement(ctx, s.db, result.job, result.resource, rid)
		if err != nil {
			return err
		}
		if good {
			s.stats.Measurements++
		}

		if result.resource.Status < 200 || result.resource.Status >= 300 {
			if err = s.issue(ctx, result.job.url, fmt.Errorf("HTTP %d", result.resource.Status)); err != nil {
				return err
			}
		}
	}

	return ctx.Err()
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
