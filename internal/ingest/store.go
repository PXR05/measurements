package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"measurements/internal/storage"
	"measurements/internal/storage/dbgen"
)

func openStore(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := storage.Open(path, false)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return nil, err
	}
	if version > 1 {
		db.Close()
		return nil, fmt.Errorf("database schema %d is newer than supported version 1", version)
	}
	if err = storage.InitSchema(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func jsonText(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func nullableID(id *int64) sql.NullInt64 {
	if id == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *id, Valid: true}
}

func saveResource(ctx context.Context, db *sql.DB, r resource) (int64, error) {
	return dbgen.New(db).UpsertResource(
		ctx,
		dbgen.UpsertResourceParams{
			URL:         r.URL,
			SHA256:      r.SHA256,
			Status:      int64(r.Status),
			FinalURL:    r.FinalURL,
			FetchedAt:   r.FetchedAt.Format(time.RFC3339Nano),
			HeadersJSON: jsonText(r.Headers),
			CachePath:   r.BodyPath, Body: r.Body,
		})
}

func saveMeasurement(ctx context.Context, db *sql.DB, job measurementJob, r resource, resourceID int64) (bool, error) {
	exists, err := dbgen.New(db).HasMeasurement(
		ctx,
		dbgen.HasMeasurementParams{
			VariantID:  job.variant,
			Channel:    job.channel,
			Sample:     job.sample,
			ResourceID: resourceID,
		})
	if err != nil || exists {
		return false, err
	}

	var points []Point
	var issues []string

	if r.Status >= 200 && r.Status < 300 {
		points, issues = ParseMeasurement(r.Body)
	} else {
		issues = []string{fmt.Sprintf("HTTP %d", r.Status)}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}

	defer tx.Rollback()
	queries := dbgen.New(tx)
	id, err := queries.CreateMeasurement(ctx, dbgen.CreateMeasurementParams{
		VariantID:       job.variant,
		Channel:         job.channel,
		Sample:          job.sample,
		ResourceID:      resourceID,
		ParseIssuesJSON: jsonText(issues),
		PointCount:      int64(len(points)),
	})
	if err != nil {
		return false, err
	}

	for i, point := range points {
		var phase sql.NullFloat64
		if point.Phase != nil {
			phase = sql.NullFloat64{
				Float64: *point.Phase,
				Valid:   true,
			}
		}
		err = queries.InsertPoint(
			ctx,
			dbgen.InsertPointParams{
				MeasurementID: id,
				Ordinal:       int64(i),
				LineNumber:    int64(point.Line),
				FrequencyHz:   point.Frequency,
				SPLDB:         point.SPL,
				PhaseDegrees:  phase,
				ValuesJSON:    jsonText(point.Values),
			},
		)
		if err != nil {
			return false, err
		}
	}

	return len(points) > 0, tx.Commit()
}
