package storage

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"measurements/internal/storage/dbgen"
)

const (
	GridSize           = 120
	minGridFrequencyHz = 20.0
	maxGridFrequencyHz = 20000.0
	maxSimilarIEMs     = 15
	batchSize          = 2000
)

var (
	gridFrequencies [GridSize]float64
	gridLogFreqs    [GridSize]float64
	normIndices     []int
	spreadIndices   []int
	simWeightsSqrt  []float32
)

func init() {
	ratio := maxGridFrequencyHz / minGridFrequencyHz
	var weightSum float64
	var rawWeights []float64
	for i := 0; i < GridSize; i++ {
		f := minGridFrequencyHz * math.Pow(ratio, float64(i)/float64(GridSize-1))
		gridFrequencies[i] = math.Round(f*100) / 100
		gridLogFreqs[i] = math.Log10(f)
		if f >= 500 && f <= 1000 {
			normIndices = append(normIndices, i)
		}
		if f <= 8000 {
			spreadIndices = append(spreadIndices, i)
		}
		if f <= 6000 {
			rawWeights = append(rawWeights, 1.0)
			weightSum += 1.0
		} else if f <= 10000 {
			rawWeights = append(rawWeights, 0.4)
			weightSum += 0.4
		}
	}
	simWeightsSqrt = make([]float32, len(rawWeights))
	for i, w := range rawWeights {
		simWeightsSqrt[i] = float32(math.Sqrt(w / weightSum))
	}
}

func GridFrequency(index int) float64 {
	return gridFrequencies[index]
}

func EncodeCurve(values []float32) []byte {
	if len(values) == 0 {
		return []byte{}
	}
	buf := make([]byte, len(values)*4)
	for i, v := range values {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func DecodeCurve(data []byte) ([]float32, bool) {
	if len(data) != GridSize*4 {
		return nil, false
	}
	values := make([]float32, GridSize)
	for i := 0; i < GridSize; i++ {
		values[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return values, true
}

type rawFreqSPL struct {
	freq float64
	spl  float64
}

func resampleMeasurement(body []byte) ([]float32, float64, bool) {
	body = bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf})
	scanner := bufio.NewScanner(bytes.NewReader(body))
	points := make([]rawFreqSPL, 0, 512)

	for scanner.Scan() {
		s := strings.TrimSpace(scanner.Text())
		if s == "" || s[0] == '*' || s[0] == '#' || s[0] == ';' || strings.HasPrefix(s, "//") {
			continue
		}
		f1, f2, ok := parseFirstTwoFloats(s)
		if !ok || f1 <= 0 || math.IsNaN(f1) || math.IsInf(f1, 0) || math.IsNaN(f2) || math.IsInf(f2, 0) {
			continue
		}
		points = append(points, rawFreqSPL{freq: f1, spl: f2})
	}
	if len(points) < 20 {
		return nil, 0, false
	}

	sort.Slice(points, func(i, j int) bool { return points[i].freq < points[j].freq })

	dedup := points[:0]
	for i := 0; i < len(points); {
		j := i + 1
		sum := points[i].spl
		for j < len(points) && points[j].freq == points[i].freq {
			sum += points[j].spl
			j++
		}
		dedup = append(dedup, rawFreqSPL{freq: points[i].freq, spl: sum / float64(j-i)})
		i = j
	}
	points = dedup

	if len(points) < 20 || points[0].freq > 100 || points[len(points)-1].freq < 6000 {
		return nil, 0, false
	}

	logFreqs := make([]float64, len(points))
	for i, p := range points {
		logFreqs[i] = math.Log10(p.freq)
	}

	interpolated := make([]float64, GridSize)
	idx := 0
	for g := 0; g < GridSize; g++ {
		targetLog := gridLogFreqs[g]
		if targetLog <= logFreqs[0] {
			interpolated[g] = points[0].spl
			continue
		}
		if targetLog >= logFreqs[len(logFreqs)-1] {
			interpolated[g] = points[len(points)-1].spl
			continue
		}
		for idx+1 < len(logFreqs) && logFreqs[idx+1] < targetLog {
			idx++
		}
		f0, f1 := logFreqs[idx], logFreqs[idx+1]
		if f1 == f0 {
			interpolated[g] = points[idx].spl
		} else {
			t := (targetLog - f0) / (f1 - f0)
			interpolated[g] = points[idx].spl + t*(points[idx+1].spl-points[idx].spl)
		}
	}

	var normSum float64
	for _, ni := range normIndices {
		normSum += interpolated[ni]
	}
	normOffset := normSum / float64(len(normIndices))

	curve := make([]float32, GridSize)
	for g := 0; g < GridSize; g++ {
		val := interpolated[g] - normOffset
		if math.Abs(val) > 60 {
			return nil, 0, false
		}
		curve[g] = float32(val)
	}
	return curve, math.Round(normOffset*100) / 100, true
}

func parseFirstTwoFloats(s string) (float64, float64, bool) {
	i := 0
	n := len(s)
	nextField := func() (string, bool) {
		for i < n && (s[i] == ' ' || s[i] == '\t' || s[i] == ',') {
			i++
		}
		if i >= n {
			return "", false
		}
		start := i
		for i < n && s[i] != ' ' && s[i] != '\t' && s[i] != ',' {
			i++
		}
		return s[start:i], true
	}
	a, ok := nextField()
	if !ok {
		return 0, 0, false
	}
	b, ok := nextField()
	if !ok {
		return 0, 0, false
	}
	f1, err1 := strconv.ParseFloat(a, 64)
	f2, err2 := strconv.ParseFloat(b, 64)
	return f1, f2, err1 == nil && err2 == nil
}

func computeMissingCurves(ctx context.Context, db dbgen.DBTX) error {
	q := dbgen.New(db)
	rows, err := q.ListUncomputedMeasurements(ctx)
	if err != nil || len(rows) == 0 {
		return err
	}

	results := make([][3]any, len(rows))
	workers := runtime.NumCPU()
	var wg sync.WaitGroup
	chunk := (len(rows) + workers - 1) / workers

	for w := 0; w < workers; w++ {
		start := w * chunk
		if start >= len(rows) {
			break
		}
		end := min(start+chunk, len(rows))
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for i := start; i < end; i++ {
				curve, offset, ok := resampleMeasurement(rows[i].Body)
				if ok {
					results[i] = [3]any{rows[i].ID, offset, hex.EncodeToString(EncodeCurve(curve))}
				} else {
					results[i] = [3]any{rows[i].ID, 0.0, ""}
				}
			}
		}(start, end)
	}
	wg.Wait()

	for i := 0; i < len(results); i += batchSize {
		end := min(i+batchSize, len(results))
		data, err := json.Marshal(results[i:end])
		if err != nil {
			return err
		}
		if err := q.UpsertMeasurementCurvesBatch(ctx, string(data)); err != nil {
			return err
		}
	}
	return nil
}

type iemRigKey struct {
	brandKey string
	iemKey   string
	rigType  string
}

type consensusEntry struct {
	key              iemRigKey
	sourceCount      int64
	measurementCount int64
	spreadDB         float64
	median           []float32
	minCurve         []float32
	maxCurve         []float32
	p25Curve         []float32
	p75Curve         []float32
	simVector        []float32
}

func RefreshAnalytics(ctx context.Context, db dbgen.DBTX) error {
	if err := computeMissingCurves(ctx, db); err != nil {
		return err
	}

	q := dbgen.New(db)
	summaries, err := q.ListCatalogIEMSummaries(ctx)
	if err != nil {
		return err
	}
	type iemMeta struct {
		brand       string
		name        string
		sourceCount int64
		priceUSD    sql.NullFloat64
	}
	metaByIEM := make(map[[2]string]iemMeta, len(summaries))
	for _, s := range summaries {
		if s.PriceUSD.Valid {
			s.PriceUSD.Float64 = math.Round(s.PriceUSD.Float64*100) / 100
		}
		metaByIEM[[2]string{s.BrandKey, s.IEMKey}] = iemMeta{
			brand:       s.Brand,
			name:        s.Name,
			sourceCount: s.SourceCount,
			priceUSD:    s.PriceUSD,
		}
	}

	mRows, err := q.ListCatalogMeasurementsForAnalytics(ctx)
	if err != nil {
		return err
	}

	type recordGroup struct {
		minVariant int64
		curves     [][]float32
	}
	grouped := make(map[iemRigKey]map[int64]*recordGroup)
	var keys []iemRigKey

	for _, r := range mRows {
		curve, ok := DecodeCurve(r.Curve)
		if !ok {
			continue
		}
		rig := "711"
		if strings.EqualFold(strings.TrimSpace(r.Type), "5128") {
			rig = "5128"
		}
		k := iemRigKey{brandKey: r.BrandKey, iemKey: r.IEMKey, rigType: rig}
		byRecord := grouped[k]
		if byRecord == nil {
			byRecord = make(map[int64]*recordGroup)
			grouped[k] = byRecord
			keys = append(keys, k)
		}
		rg := byRecord[r.RecordID]
		if rg == nil {
			rg = &recordGroup{minVariant: r.VariantOrdinal}
			byRecord[r.RecordID] = rg
		}
		if r.VariantOrdinal == rg.minVariant {
			rg.curves = append(rg.curves, curve)
		}
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].rigType != keys[j].rigType {
			return keys[i].rigType < keys[j].rigType
		}
		if keys[i].brandKey != keys[j].brandKey {
			return keys[i].brandKey < keys[j].brandKey
		}
		return keys[i].iemKey < keys[j].iemKey
	})

	entries := make([]consensusEntry, 0, len(keys))
	byRig := map[string][]int{"711": {}, "5128": {}}

	for _, k := range keys {
		byRecord := grouped[k]
		sourceCurves := make([][]float32, 0, len(byRecord))
		var allCurves [][]float32

		for _, rg := range byRecord {
			if len(rg.curves) == 0 {
				continue
			}
			allCurves = append(allCurves, rg.curves...)
			mean := make([]float32, GridSize)
			for _, c := range rg.curves {
				for g := 0; g < GridSize; g++ {
					mean[g] += c[g]
				}
			}
			inv := 1.0 / float32(len(rg.curves))
			for g := 0; g < GridSize; g++ {
				mean[g] *= inv
			}
			sourceCurves = append(sourceCurves, mean)
		}
		if len(sourceCurves) == 0 {
			continue
		}

		median := make([]float32, GridSize)
		minCurve := make([]float32, GridSize)
		maxCurve := make([]float32, GridSize)
		p25Curve := make([]float32, GridSize)
		p75Curve := make([]float32, GridSize)

		colSources := make([]float32, len(sourceCurves))
		colAll := make([]float32, len(allCurves))

		for g := 0; g < GridSize; g++ {
			for i, sc := range sourceCurves {
				colSources[i] = sc[g]
			}
			sortFloats(colSources)
			median[g] = percentileSorted(colSources, 0.50)

			for i, ac := range allCurves {
				colAll[i] = ac[g]
			}
			sortFloats(colAll)
			minCurve[g] = colAll[0]
			maxCurve[g] = colAll[len(colAll)-1]

			if len(colSources) >= 2 {
				p25Curve[g] = percentileSorted(colSources, 0.25)
				p75Curve[g] = percentileSorted(colSources, 0.75)
			} else {
				p25Curve[g] = percentileSorted(colAll, 0.25)
				p75Curve[g] = percentileSorted(colAll, 0.75)
			}
		}

		var spreadSum float64
		for _, si := range spreadIndices {
			spreadSum += float64(maxCurve[si] - minCurve[si])
		}
		spreadDB := math.Round((spreadSum/float64(len(spreadIndices)))*100) / 100

		simVec := buildSimVector(median)
		idx := len(entries)
		entries = append(entries, consensusEntry{
			key:              k,
			sourceCount:      int64(len(sourceCurves)),
			measurementCount: int64(len(allCurves)),
			spreadDB:         spreadDB,
			median:           median,
			minCurve:         minCurve,
			maxCurve:         maxCurve,
			p25Curve:         p25Curve,
			p75Curve:         p75Curve,
			simVector:        simVec,
		})
		byRig[k.rigType] = append(byRig[k.rigType], idx)
	}

	if err := q.ClearIEMConsensus(ctx); err != nil {
		return err
	}
	consensusBatch := make([][11]any, len(entries))
	for i, e := range entries {
		consensusBatch[i] = [11]any{
			e.key.brandKey,
			e.key.iemKey,
			e.key.rigType,
			e.sourceCount,
			e.measurementCount,
			e.spreadDB,
			hex.EncodeToString(EncodeCurve(e.median)),
			hex.EncodeToString(EncodeCurve(e.minCurve)),
			hex.EncodeToString(EncodeCurve(e.maxCurve)),
			hex.EncodeToString(EncodeCurve(e.p25Curve)),
			hex.EncodeToString(EncodeCurve(e.p75Curve)),
		}
	}
	for i := 0; i < len(consensusBatch); i += 500 {
		end := min(i+500, len(consensusBatch))
		data, err := json.Marshal(consensusBatch[i:end])
		if err != nil {
			return err
		}
		if err := q.InsertIEMConsensusBatch(ctx, string(data)); err != nil {
			return err
		}
	}

	if err := q.ClearIEMSimilarities(ctx); err != nil {
		return err
	}

	type neighbor struct {
		idx  int
		dist float32
	}
	var simRows [][11]any
	for _, indices := range byRig {
		n := len(indices)
		if n < 2 {
			continue
		}
		topNeighbors := make([][]neighbor, n)
		workers := runtime.NumCPU()
		var wg sync.WaitGroup
		chunk := (n + workers - 1) / workers

		for w := 0; w < workers; w++ {
			start := w * chunk
			if start >= n {
				break
			}
			end := min(start+chunk, n)
			wg.Add(1)
			go func(start, end int) {
				defer wg.Done()
				for a := start; a < end; a++ {
					vecA := entries[indices[a]].simVector
					best := make([]neighbor, 0, maxSimilarIEMs+1)
					var worstDist float32 = math.MaxFloat32

					for b := 0; b < n; b++ {
						if a == b {
							continue
						}
						vecB := entries[indices[b]].simVector
						var sumSq float32
						for k := 0; k < len(vecA); k++ {
							d := vecA[k] - vecB[k]
							sumSq += d * d
						}
						if len(best) == maxSimilarIEMs && sumSq >= worstDist {
							continue
						}
						pos := len(best)
						for pos > 0 && best[pos-1].dist > sumSq {
							pos--
						}
						if len(best) < maxSimilarIEMs {
							best = append(best, neighbor{})
						}
						copy(best[pos+1:], best[pos:])
						best[pos] = neighbor{idx: indices[b], dist: sumSq}
						if len(best) == maxSimilarIEMs {
							worstDist = best[maxSimilarIEMs-1].dist
						}
					}
					topNeighbors[a] = best
				}
			}(start, end)
		}
		wg.Wait()

		for a, neighbors := range topNeighbors {
			src := entries[indices[a]]
			for rank, nb := range neighbors {
				target := entries[nb.idx]
				meta := metaByIEM[[2]string{target.key.brandKey, target.key.iemKey}]
				distDB := math.Round(math.Sqrt(float64(nb.dist))*100) / 100
				var price any
				if meta.priceUSD.Valid {
					price = meta.priceUSD.Float64
				}
				simRows = append(simRows, [11]any{
					src.key.brandKey,
					src.key.iemKey,
					src.key.rigType,
					rank + 1,
					target.key.brandKey,
					target.key.iemKey,
					meta.brand,
					meta.name,
					meta.sourceCount,
					price,
					distDB,
				})
			}
		}
	}

	for i := 0; i < len(simRows); i += batchSize {
		end := min(i+batchSize, len(simRows))
		data, err := json.Marshal(simRows[i:end])
		if err != nil {
			return err
		}
		if err := q.InsertIEMSimilaritiesBatch(ctx, string(data)); err != nil {
			return err
		}
	}
	return nil
}

func buildSimVector(curve []float32) []float32 {
	m := len(simWeightsSqrt)
	var weightedMean float32
	for i := 0; i < m; i++ {
		w := simWeightsSqrt[i] * simWeightsSqrt[i]
		weightedMean += curve[i] * w
	}
	vec := make([]float32, m)
	for i := 0; i < m; i++ {
		vec[i] = (curve[i] - weightedMean) * simWeightsSqrt[i]
	}
	return vec
}

func sortFloats(a []float32) {
	for i := 1; i < len(a); i++ {
		v := a[i]
		j := i - 1
		for j >= 0 && a[j] > v {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = v
	}
}

func percentileSorted(sorted []float32, p float64) float32 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	pos := p * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := float32(pos - float64(lo))
	return sorted[lo] + frac*(sorted[hi]-sorted[lo])
}
