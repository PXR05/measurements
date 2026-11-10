package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"measurements/internal/storage"
)

type Key struct {
	Brand string `json:"brand"`
	Name  string `json:"name"`
}

func (k Key) String() string {
	if k.Name == "" {
		return k.Brand
	}
	return k.Brand + " / " + k.Name
}

type Decision struct {
	ID    int64  `json:"id"`
	Scope string `json:"scope,omitempty"`
	Kind  string `json:"kind"`
	Left  Key    `json:"left"`
	Right Key    `json:"right"`
}

type Backup struct {
	Version   int        `json:"version"`
	Decisions []Decision `json:"decisions"`
}

type Store struct{ DB *sql.DB }

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := storage.Open(path, false)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	var version int
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err == nil && version != 1 {
		err = fmt.Errorf("expected an existing schema 1 seed database; run cmd/seed first")
	}
	if err == nil {
		err = storage.InstallViews(ctx, db)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{DB: db}, nil
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func decisions(ctx context.Context, q queryer) ([]Decision, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,kind,left_brand,left_name,right_brand,right_name FROM iem_identity_decisions ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Decision{}
	for rows.Next() {
		var d Decision
		if err := rows.Scan(&d.ID, &d.Kind, &d.Left.Brand, &d.Left.Name, &d.Right.Brand, &d.Right.Name); err != nil {
			return nil, err
		}
		if d.Left.Name == "" && d.Right.Name == "" {
			d.Scope = "brand"
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (s *Store) Decisions(ctx context.Context) ([]Decision, error) { return decisions(ctx, s.DB) }

func activeDismissals(ds []Decision) []Decision {
	latest := make(map[[2]Key]string)
	pair := func(d Decision) [2]Key {
		a, b := d.Left, d.Right
		if keyLess(b, a) {
			a, b = b, a
		}
		return [2]Key{a, b}
	}
	for _, d := range ds {
		latest[pair(d)] = d.Kind
	}
	result := []Decision{}
	for _, d := range ds {
		if d.Kind == "dismiss" && latest[pair(d)] == "dismiss" {
			result = append(result, d)
		}
	}
	return result
}

type roots map[Key]Key

func (r roots) find(k Key) Key {
	if k.Name != "" {
		k.Brand = r.find(Key{Brand: k.Brand}).Brand
	}
	if parent, ok := r[k]; ok && parent != k {
		r[k] = r.find(parent)
		return r[k]
	}
	return k
}

func replay(ds []Decision) (roots, error) {
	r := roots{}
	for _, d := range ds {
		brand := d.Left.Name == "" && d.Right.Name == ""
		if (d.Left.Name == "") != (d.Right.Name == "") || d.Left == d.Right ||
			(brand && (d.Left.Brand == "" || d.Right.Brand == "")) ||
			(d.Scope != "" && d.Scope != "iem" && d.Scope != "brand") ||
			(d.Scope == "brand" && !brand) || (d.Scope == "iem" && brand) ||
			d.Left.Brand != normalizeKey(d.Left.Brand) ||
			d.Right.Brand != normalizeKey(d.Right.Brand) ||
			d.Left.Name != normalizeKey(d.Left.Name) ||
			d.Right.Name != normalizeKey(d.Right.Name) {
			return nil, errors.New("invalid identity keys")
		}
		if d.Kind != "merge" && d.Kind != "dismiss" {
			return nil, errors.New("decision must be merge or dismiss")
		}
		if brand && d.Kind == "merge" {
			a, b := r.find(d.Left), r.find(d.Right)
			if a != b {
				r[a] = b
			}
		}
	}
	for _, d := range ds {
		if d.Left.Name == "" {
			continue
		}
		switch d.Kind {
		case "merge":
			a, b := r.find(d.Left), r.find(d.Right)
			if a != b {
				r[a] = b
			}
		case "dismiss":
		default:
			return nil, errors.New("decision must be merge or dismiss")
		}
	}
	for _, d := range activeDismissals(ds) {
		if r.find(d.Left) == r.find(d.Right) {
			return nil, fmt.Errorf("blocked by dismissal #%d: %s ≠ %s", d.ID, d.Left, d.Right)
		}
	}
	for k := range r {
		r[k] = r.find(k)
	}
	return r, nil
}

func normalizeKey(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, strings.Trim(s, " \t\r\n"))
}

func rebuild(ctx context.Context, tx *sql.Tx) error {
	ds, err := decisions(ctx, tx)
	if err != nil {
		return err
	}
	r, err := replay(ds)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM iem_identity_aliases"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM brand_identity_aliases"); err != nil {
		return err
	}
	for k, v := range r {
		if k == v {
			continue
		}
		if k.Name == "" {
			if _, err = tx.ExecContext(ctx, `INSERT INTO brand_identity_aliases VALUES(?,?)`, k.Brand, v.Brand); err != nil {
				return err
			}
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO iem_identity_aliases VALUES(?,?,?,?)`, k.Brand, k.Name, v.Brand, v.Name); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Decide(ctx context.Context, d Decision) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ds, err := decisions(ctx, tx)
	if err != nil {
		return err
	}
	r, err := replay(ds)
	if err != nil {
		return err
	}
	if r.find(d.Left) == r.find(d.Right) {
		return errors.New("these identities are already merged; refresh the queue")
	}
	for _, k := range []Key{d.Left, d.Right} {
		var exists bool
		query := `SELECT EXISTS(SELECT 1 FROM api_current_phone_catalog WHERE brand_key=? AND iem_key=?)`
		args := []any{k.Brand, k.Name}
		if k.Name == "" {
			query = `SELECT EXISTS(SELECT 1 FROM api_raw_phone_catalog p LEFT JOIN brand_identity_aliases a ON a.brand_key=p.brand_key WHERE coalesce(a.canonical_brand,p.brand_key)=?)`
			args = []any{k.Brand}
		}
		if err = tx.QueryRowContext(ctx, query, args...).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return errors.New("identity is no longer in the current catalog; refresh the queue")
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO iem_identity_decisions(kind,left_brand,left_name,right_brand,right_name) VALUES(?,?,?,?,?)`, d.Kind, d.Left.Brand, d.Left.Name, d.Right.Brand, d.Right.Name); err != nil {
		return err
	}
	if err = rebuild(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Undo(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM iem_identity_decisions WHERE id=(SELECT max(id) FROM iem_identity_decisions)`); err != nil {
		return err
	}
	if err = rebuild(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Restore(ctx context.Context, b Backup) error {
	if b.Version != 1 && b.Version != 2 {
		return errors.New("unsupported backup version")
	}
	if _, err := replay(b.Decisions); err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM iem_identity_decisions").Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return errors.New("restore requires an empty decision log; existing reviews were kept")
	}
	for _, d := range b.Decisions {
		if _, err = tx.ExecContext(ctx, `INSERT INTO iem_identity_decisions(kind,left_brand,left_name,right_brand,right_name) VALUES(?,?,?,?,?)`, d.Kind, d.Left.Brand, d.Left.Name, d.Right.Brand, d.Right.Name); err != nil {
			return err
		}
	}
	if err = rebuild(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}
