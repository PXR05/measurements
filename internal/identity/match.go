package identity

import (
	"context"
	"sort"
	"strings"
	"unicode"
)

type Record struct {
	Brand  string `json:"brand"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Type   string `json:"type"`
	ID     int64  `json:"record_id"`
}
type Product struct {
	Key         Key      `json:"key"`
	Records     []Record `json:"records"`
	RecordCount int      `json:"record_count,omitempty"`
	ModelCount  int      `json:"model_count,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
}
type Candidate struct {
	Left   Product `json:"left"`
	Right  Product `json:"right"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}
type catalogRecord struct {
	key    Key
	record Record
}

func (s *Store) catalog(ctx context.Context) ([]catalogRecord, error) {
	return catalog(ctx, s.DB)
}

func catalog(ctx context.Context, q queryer) ([]catalogRecord, error) {
	rows, err := q.QueryContext(ctx, `SELECT brand_key,iem_key,brand,name,source,type,record_id FROM api_raw_phone_catalog ORDER BY brand_key,iem_key,source,record_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []catalogRecord{}
	for rows.Next() {
		var r catalogRecord
		if err = rows.Scan(&r.key.Brand, &r.key.Name, &r.record.Brand, &r.record.Name, &r.record.Source, &r.record.Type, &r.record.ID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func products(records []catalogRecord, ds []Decision) ([]Product, roots, error) {
	r, err := replay(ds)
	if err != nil {
		return nil, nil, err
	}
	byKey := map[Key]*Product{}
	for _, record := range records {
		k := r.find(record.key)
		if byKey[k] == nil {
			byKey[k] = &Product{Key: k, Records: []Record{}}
		}
		byKey[k].Records = append(byKey[k].Records, record.record)
	}
	result := make([]Product, 0, len(byKey))
	for _, p := range byKey {
		result = append(result, *p)
	}
	sort.Slice(result, func(i, j int) bool { return keyLess(result[i].Key, result[j].Key) })
	return result, r, nil
}

func keyLess(a, b Key) bool {
	if a.Brand != b.Brand {
		return a.Brand < b.Brand
	}
	return a.Name < b.Name
}

func compact(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}
func model(brand, name string) string {
	return autoSignature(name, []string{brand})
}

func similarity(a, b string) float64 {
	ar, br := []rune(a), []rune(b)
	n, m := len(ar), len(br)
	if n == 0 || m == 0 {
		return 0
	}
	if a == b {
		return 1
	}
	if float64(min(n, m))/float64(max(n, m)) < .55 {
		return 0
	}
	prev := make([]int, m+1)
	for j := range prev {
		prev[j] = j
	}
	for i, x := range ar {
		diagonal := prev[0]
		prev[0] = i + 1
		for j, y := range br {
			old := prev[j+1]
			cost := 1
			if x == y {
				cost = 0
			}
			prev[j+1] = min(prev[j]+1, old+1, diagonal+cost)
			diagonal = old
		}
	}
	return 1 - float64(prev[m])/float64(max(n, m))
}

func candidates(items []Product, ds []Decision, r roots) []Candidate {
	if len(items) > 0 && items[0].Key.Name == "" {
		return brandCandidates(items, ds, r)
	}
	index := map[Key]int{}
	for i, p := range items {
		index[p.Key] = i
	}
	dismissed := map[[2]int]bool{}
	for _, d := range activeDismissals(ds) {
		a, oka := index[r.find(d.Left)]
		b, okb := index[r.find(d.Right)]
		if oka && okb {
			dismissed[[2]int{min(a, b), max(a, b)}] = true
		}
	}
	brands, models := map[string]map[int]bool{}, map[string]map[int]bool{}
	names := make([]map[string]bool, len(items))
	for i, p := range items {
		names[i] = map[string]bool{}
		canonicalBrand := compact(p.Key.Brand)
		if canonicalBrand != "" {
			if brands[canonicalBrand] == nil {
				brands[canonicalBrand] = map[int]bool{}
			}
			brands[canonicalBrand][i] = true
		}
		for _, rec := range p.Records {
			b, n := compact(rec.Brand), model(rec.Brand, rec.Name)
			names[i][n] = true
			if b != "" {
				if brands[b] == nil {
					brands[b] = map[int]bool{}
				}
				brands[b][i] = true
			}
			if len([]rune(n)) >= 4 {
				if models[n] == nil {
					models[n] = map[int]bool{}
				}
				models[n][i] = true
			}
		}
	}
	seen := map[[2]int]bool{}
	result := []Candidate{}
	compare := func(bucket map[int]bool, crossBrand bool) {
		ids := make([]int, 0, len(bucket))
		for i := range bucket {
			ids = append(ids, i)
		}
		sort.Ints(ids)
		for x, a := range ids {
			for _, b := range ids[x+1:] {
				pair := [2]int{a, b}
				if seen[pair] || dismissed[pair] {
					continue
				}
				seen[pair] = true
				score := 0.0
				for an := range names[a] {
					for bn := range names[b] {
						score = max(score, similarity(an, bn))
					}
				}
				if score < .55 {
					continue
				}
				reason := "Similar model names within a normalized brand"
				if score == 1 {
					reason = "Same model name after removing brand prefix, spaces and punctuation"
				}
				if crossBrand {
					reason = "Matching model name across different brand spellings — check brand carefully"
					score = min(score, .95)
				}
				result = append(result, Candidate{Left: items[a], Right: items[b], Score: score, Reason: reason})
			}
		}
	}
	keys := make([]string, 0, len(brands))
	for k := range brands {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		compare(brands[k], false)
	}
	keys = keys[:0]
	for k := range models {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		compare(models[k], true)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		if result[i].Left.Key != result[j].Left.Key {
			return keyLess(result[i].Left.Key, result[j].Left.Key)
		}
		return keyLess(result[i].Right.Key, result[j].Right.Key)
	})
	return result
}

func brandProducts(records []catalogRecord, ds []Decision) ([]Product, roots, error) {
	r, err := replay(ds)
	if err != nil {
		return nil, nil, err
	}
	groups := map[Key]*Product{}
	models := map[Key]map[string]bool{}
	aliases := map[Key]map[string]bool{}
	for _, rec := range records {
		k := r.find(Key{Brand: rec.key.Brand})
		if k.Brand == "" {
			continue
		}
		if groups[k] == nil {
			groups[k] = &Product{Key: k, Records: []Record{}}
			models[k] = map[string]bool{}
			aliases[k] = map[string]bool{}
		}
		p := groups[k]
		p.RecordCount++
		models[k][rec.key.Name] = true
		aliases[k][rec.record.Brand] = true
		if len(p.Records) < 12 {
			p.Records = append(p.Records, rec.record)
		}
	}
	result := make([]Product, 0, len(groups))
	for k, p := range groups {
		p.ModelCount = len(models[k])
		for alias := range aliases[k] {
			p.Aliases = append(p.Aliases, alias)
		}
		sort.Strings(p.Aliases)
		result = append(result, *p)
	}
	sort.Slice(result, func(i, j int) bool { return keyLess(result[i].Key, result[j].Key) })
	return result, r, nil
}

func brandCandidates(items []Product, ds []Decision, r roots) []Candidate {
	blocked := map[[2]Key]bool{}
	for _, d := range activeDismissals(ds) {
		if d.Left.Name != "" {
			continue
		}
		a, b := r.find(d.Left), r.find(d.Right)
		if keyLess(b, a) {
			a, b = b, a
		}
		blocked[[2]Key{a, b}] = true
	}
	names := make([]map[string]bool, len(items))
	for i, p := range items {
		names[i] = map[string]bool{compact(p.Key.Brand): true}
		for _, alias := range p.Aliases {
			names[i][compact(alias)] = true
		}
	}
	result := []Candidate{}
	for i, a := range items {
		for j := i + 1; j < len(items); j++ {
			b := items[j]
			if blocked[[2]Key{a.Key, b.Key}] {
				continue
			}
			score := 0.0
			for an := range names[i] {
				for bn := range names[j] {
					score = max(score, similarity(an, bn))
				}
			}
			if score < .55 {
				continue
			}
			reason := "Similar brand names"
			if score == 1 {
				reason = "Same brand spelling after removing spaces and punctuation"
			}
			result = append(result, Candidate{Left: a, Right: b, Score: score, Reason: reason})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		if result[i].Left.Key != result[j].Left.Key {
			return keyLess(result[i].Left.Key, result[j].Left.Key)
		}
		return keyLess(result[i].Right.Key, result[j].Right.Key)
	})
	return result
}
