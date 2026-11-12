package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"measurements/internal/storage"
	"measurements/internal/storage/dbgen"
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
		err = storage.InitSchema(ctx, db)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{DB: db}, nil
}

func decisions(ctx context.Context, db dbgen.DBTX) ([]Decision, error) {
	rows, err := dbgen.New(db).ListIdentityDecisions(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Decision, 0, len(rows))
	for _, r := range rows {
		d := Decision{
			ID:    r.ID,
			Kind:  r.Kind,
			Left:  Key{Brand: r.LeftBrand, Name: r.LeftName},
			Right: Key{Brand: r.RightBrand, Name: r.RightName},
		}
		if d.Left.Name == "" && d.Right.Name == "" {
			d.Scope = "brand"
		}
		result = append(result, d)
	}
	return result, nil
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
	q := dbgen.New(tx)
	if err = q.ClearIEMAliases(ctx); err != nil {
		return err
	}
	if err = q.ClearBrandAliases(ctx); err != nil {
		return err
	}
	for k, v := range r {
		if k == v {
			continue
		}
		if k.Name == "" {
			if err = q.InsertBrandAlias(ctx, dbgen.InsertBrandAliasParams{
				BrandKey:       k.Brand,
				CanonicalBrand: v.Brand,
			}); err != nil {
				return err
			}
			continue
		}
		if err = q.InsertIEMAlias(ctx, dbgen.InsertIEMAliasParams{
			BrandKey:       k.Brand,
			IEMKey:         k.Name,
			CanonicalBrand: v.Brand,
			CanonicalName:  v.Name,
		}); err != nil {
			return err
		}
	}
	return storage.RefreshCatalog(ctx, tx)
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
	q := dbgen.New(tx)
	for _, k := range []Key{d.Left, d.Right} {
		var exists bool
		if k.Name == "" {
			exists, err = q.BrandExists(ctx, k.Brand)
		} else {
			exists, err = q.IEMExists(ctx, dbgen.IEMExistsParams{BrandKey: k.Brand, IEMKey: k.Name})
		}
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("identity is no longer in the current catalog; refresh the queue")
		}
	}
	if err = q.InsertIdentityDecision(ctx, dbgen.InsertIdentityDecisionParams{
		Kind:       d.Kind,
		LeftBrand:  d.Left.Brand,
		LeftName:   d.Left.Name,
		RightBrand: d.Right.Brand,
		RightName:  d.Right.Name,
	}); err != nil {
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
	if err = dbgen.New(tx).DeleteLatestIdentityDecision(ctx); err != nil {
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
	q := dbgen.New(tx)
	count, err := q.CountIdentityDecisions(ctx)
	if err != nil {
		return err
	}
	if count != 0 {
		return errors.New("restore requires an empty decision log; existing reviews were kept")
	}
	for _, d := range b.Decisions {
		if err = q.InsertIdentityDecision(ctx, dbgen.InsertIdentityDecisionParams{
			Kind:       d.Kind,
			LeftBrand:  d.Left.Brand,
			LeftName:   d.Left.Name,
			RightBrand: d.Right.Brand,
			RightName:  d.Right.Name,
		}); err != nil {
			return err
		}
	}
	if err = rebuild(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}
