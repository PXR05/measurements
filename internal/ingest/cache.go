package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type resource struct {
	URL       string      `json:"url"`
	FinalURL  string      `json:"final_url"`
	Status    int         `json:"status"`
	Headers   http.Header `json:"headers"`
	FetchedAt time.Time   `json:"fetched_at"`
	SHA256    string      `json:"sha256"`
	BodyPath  string      `json:"body_path"`
	Body      []byte      `json:"-"`
}

type cache struct {
	root             string
	client           *http.Client
	offline, refresh bool
	writeMu          sync.Mutex
}

type cacheError struct{ error }

func (c *cache) persist(r resource, index string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	object := filepath.Join(c.root, r.BodyPath)
	old, err := os.ReadFile(object)
	if os.IsNotExist(err) {
		err = atomicWrite(object, r.Body)
	} else if err == nil && digest(old) != r.SHA256 {
		err = fmt.Errorf("cache checksum mismatch: %s", object)
	}
	if err != nil {
		return cacheError{err}
	}

	meta, _ := json.MarshalIndent(r, "", "  ")
	archive := filepath.Join(
		c.root,
		"responses",
		digest([]byte(r.URL)),
		r.FetchedAt.Format("20060102T150405.000000000")+".json",
	)
	if err = atomicWrite(archive, meta); err != nil {
		return cacheError{err}
	}
	if err = atomicWrite(index, meta); err != nil {
		return cacheError{err}
	}

	return nil
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	f, err := os.CreateTemp(filepath.Dir(path), ".ingest-*")
	if err != nil {
		return err
	}

	defer os.Remove(f.Name())

	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}

	if err = f.Close(); err != nil {
		return err
	}

	return os.Rename(f.Name(), path)
}

func (c *cache) fetch(ctx context.Context, address string) (resource, error) {
	index := filepath.Join(c.root, "urls", digest([]byte(address))+".json")

	if !c.refresh || c.offline {
		data, err := os.ReadFile(index)

		if err == nil {
			var r resource
			if err = json.Unmarshal(data, &r); err != nil {
				return r, cacheError{err}
			}

			r.Body, err = os.ReadFile(filepath.Join(c.root, r.BodyPath))
			if err != nil {
				return r, cacheError{err}
			}

			if digest(r.Body) != r.SHA256 {
				return r, cacheError{fmt.Errorf("cache checksum mismatch: %s", address)}
			}

			return r, nil
		}

		if !os.IsNotExist(err) {
			return resource{}, cacheError{err}
		}
	}

	if c.offline {
		return resource{}, fmt.Errorf("not cached: %s", address)
	}

	var r resource
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		if err != nil {
			return r, err
		}

		req.Header.Set("User-Agent", "measurements-seeder/1.0")
		resp, err := c.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return r, ctx.Err()
			}

			if attempt == 2 {
				return r, err
			}
		} else {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				return r, readErr
			}

			r = resource{
				URL:       address,
				FinalURL:  resp.Request.URL.String(),
				Status:    resp.StatusCode,
				Headers:   resp.Header.Clone(),
				FetchedAt: time.Now().UTC(),
				SHA256:    digest(body),
				Body:      body,
			}
			r.BodyPath = filepath.Join("objects", r.SHA256+".body")

			if err = c.persist(r, index); err != nil {
				return r, err
			}

			if resp.StatusCode != 429 && resp.StatusCode < 500 || attempt == 2 {
				return r, nil
			}
		}

		timer := time.NewTimer(time.Duration(attempt+1) * time.Second)

		select {
		case <-ctx.Done():
			timer.Stop()
			return r, ctx.Err()
		case <-timer.C:
		}
	}
	return r, nil
}
