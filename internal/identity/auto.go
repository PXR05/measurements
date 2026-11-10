package identity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

var numbers = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)*`)
var negative = regexp.MustCompile(`^-\s*[0-9]+(?:\.[0-9]+)?(?:\s*(?:db|khz|hz|ohm|Ω|%)|\s|$|[)\]}])`)

func unwrapName(s string) string {
	s = strings.TrimSpace(s)
	var value any
	if json.Unmarshal([]byte(s), &value) == nil {
		switch v := value.(type) {
		case string:
			return strings.TrimSpace(v)
		case []any:
			if len(v) == 1 {
				if name, ok := v[0].(string); ok {
					return strings.TrimSpace(name)
				}
			}
		}
	}
	if len(s) > 1 && s[0] == s[len(s)-1] && (s[0] == '\'' || s[0] == '"') {
		return strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

func typography(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 0xff01 && r <= 0xff5e {
			r -= 0xfee0
		}
		switch r {
		case '−', '–', '—', '‐', '‑':
			r = '-'
		case '‘', '’':
			r = '\''
		case '“', '”':
			r = '"'
		}
		return unicode.ToLower(r)
	}, s)
}

func autoSignature(name string, brands []string) string {
	s := strings.TrimSpace(typography(unwrapName(name)))
	for _, brand := range brands {
		brand = typography(brand)
		if tail, ok := strings.CutPrefix(s, brand); ok && tail != "" {
			first := []rune(tail)[0]
			if unicode.IsSpace(first) || strings.ContainsRune("-:/", first) {
				s = strings.TrimLeft(tail, " \t\r\n-:/")
				break
			}
		}
	}
	digits := numbers.FindAllString(s, -1)
	runes := []rune(s)
	var out strings.Builder
	for i, r := range runes {
		if r == '-' {
			boundary := i == 0 || unicode.IsSpace(runes[i-1]) || strings.ContainsRune("(:=", runes[i-1])
			trailing := strings.TrimSpace(string(runes[i+1:])) == "" || strings.ContainsRune(")]}", runes[min(i+1, len(runes)-1)])
			if trailing || (boundary && negative.MatchString(string(runes[i:]))) {
				out.WriteRune('−')
			}
			continue
		}
		if r == '.' && i > 0 && i+1 < len(runes) && unicode.IsDigit(runes[i-1]) && unicode.IsDigit(runes[i+1]) {
			out.WriteRune(r)
			continue
		}
		if unicode.IsSpace(r) || strings.ContainsRune(`._:()[]{}"'&/#`, r) {
			continue
		}
		out.WriteRune(r)
	}
	if out.Len() == 0 {
		return ""
	}
	return out.String() + "\x00" + strings.Join(digits, ",")
}

func cleanerName(a, b string) bool {
	aw, bw := unwrapName(a) != a, unwrapName(b) != b
	if aw != bw {
		return !aw
	}
	count := func(s string) int {
		n := 0
		for _, r := range s {
			if strings.ContainsRune(`.-_:()[]{}"'&/#‘’“”‐‑–—`, r) {
				n++
			}
		}
		return n
	}
	if ac, bc := count(a), count(b); ac != bc {
		return ac < bc
	}
	return a < b
}

func autoDecisions(records []catalogRecord, ds []Decision) ([]Decision, error) {
	items, roots, err := products(records, ds)
	if err != nil {
		return nil, err
	}
	aliases := map[string]map[string]bool{}
	add := func(brand, alias string) {
		if alias == "" {
			return
		}
		if aliases[brand] == nil {
			aliases[brand] = map[string]bool{}
		}
		aliases[brand][alias] = true
	}
	for _, record := range records {
		b := roots.find(Key{Brand: record.key.Brand}).Brand
		add(b, b)
		add(b, record.record.Brand)
	}
	for _, d := range ds {
		if d.Left.Name == "" {
			b := roots.find(d.Left).Brand
			add(b, d.Left.Brand)
			add(b, d.Right.Brand)
		}
	}
	prefixes := map[string][]string{}
	for b, names := range aliases {
		for name := range names {
			prefixes[b] = append(prefixes[b], name)
		}
		sort.Slice(prefixes[b], func(i, j int) bool {
			a, c := prefixes[b][i], prefixes[b][j]
			if len(a) != len(c) {
				return len(a) > len(c)
			}
			return a < c
		})
	}
	buckets := map[Key][]Product{}
	for _, p := range items {
		if sig := autoSignature(p.Key.Name, prefixes[p.Key.Brand]); sig != "" {
			key := Key{p.Key.Brand, sig}
			buckets[key] = append(buckets[key], p)
		}
	}
	keys := make([]Key, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keyLess(keys[i], keys[j]) })
	sources := func(p Product) int {
		set := map[string]bool{}
		for _, r := range p.Records {
			set[r.Source] = true
		}
		return len(set)
	}
	result := []Decision{}
	dismissals := activeDismissals(ds)
	for _, key := range keys {
		group := buckets[key]
		if len(group) < 2 {
			continue
		}
		members := map[Key]bool{}
		for _, p := range group {
			members[p.Key] = true
		}
		blocked := false
		for _, d := range dismissals {
			if members[roots.find(d.Left)] && members[roots.find(d.Right)] {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		sort.Slice(group, func(i, j int) bool {
			a, b := group[i], group[j]
			if x, y := sources(a), sources(b); x != y {
				return x > y
			}
			if len(a.Records) != len(b.Records) {
				return len(a.Records) > len(b.Records)
			}
			return cleanerName(a.Key.Name, b.Key.Name)
		})
		for _, p := range group[1:] {
			result = append(result, Decision{Scope: "iem", Kind: "merge", Left: p.Key, Right: group[0].Key})
		}
	}
	return result, nil
}

func (s *Store) AutoMerge(ctx context.Context, backupDir string) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	ds, err := decisions(ctx, tx)
	if err != nil {
		return 0, err
	}
	records, err := catalog(ctx, tx)
	if err != nil {
		return 0, err
	}
	merges, err := autoDecisions(records, ds)
	if err != nil || len(merges) == 0 {
		return 0, err
	}
	if _, err = replay(append(append([]Decision{}, ds...), merges...)); err != nil {
		return 0, err
	}
	if err = os.MkdirAll(backupDir, 0700); err != nil {
		return 0, err
	}
	name := filepath.Join(backupDir, "decisions-"+time.Now().UTC().Format("20060102T150405.000000000Z")+".json")
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return 0, err
	}
	err = json.NewEncoder(f).Encode(Backup{Version: 2, Decisions: ds})
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return 0, err
	}
	for _, d := range merges {
		if _, err = tx.ExecContext(ctx, `INSERT INTO iem_identity_decisions(kind,left_brand,left_name,right_brand,right_name) VALUES(?,?,?,?,?)`, d.Kind, d.Left.Brand, d.Left.Name, d.Right.Brand, d.Right.Name); err != nil {
			return 0, err
		}
	}
	if err = rebuild(ctx, tx); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(merges), nil
}
