package ingest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func DataURL(site SquigSite, db SquigDB) (string, error) {
	base := "https://squig.link"

	switch site.URLType {
	case "root":
	case "subdomain":
		if !regexp.MustCompile(`^[a-zA-Z0-9-]+$`).MatchString(site.Username) {
			return "", fmt.Errorf("invalid subdomain %q", site.Username)
		}
		base = "https://" + site.Username + ".squig.link"
	case "altDomain":
		base = site.AltDomain
	case "labFolder":
		base += "/lab/" + url.PathEscape(site.Username)
	default:
		return "", fmt.Errorf("unknown urlType %q", site.URLType)
	}

	url, err := url.Parse(base)
	if err != nil || url.Host == "" || (url.Scheme != "https" && url.Scheme != "http") {
		return "", fmt.Errorf("invalid site URL %q", base)
	}

	url.Path = strings.TrimRight(url.Path, "/") + "/" + strings.Trim(db.Folder, "/")
	url.Path = strings.TrimRight(url.Path, "/") + "/data/"
	url.RawPath = ""
	url.RawQuery = ""
	url.Fragment = ""
	return url.String(), nil
}

func fileURL(base, filename string) string {
	u, _ := url.Parse(base)
	u.Path += filename
	u.RawPath = ""
	return u.String()
}

type Point struct {
	Line           int
	Frequency, SPL float64
	Phase          *float64
	Values         []float64
}

func ParseMeasurement(data []byte) ([]Point, []string) {
	var points []Point
	var issues []string

	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	scanner := bufio.NewScanner(bytes.NewReader(data))

	line := 0
	for scanner.Scan() {
		line++

		s := strings.TrimSpace(scanner.Text())
		if s == "" || strings.HasPrefix(s, "*") || strings.HasPrefix(s, "#") || strings.HasPrefix(s, ";") || strings.HasPrefix(s, "//") {
			continue
		}

		fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
		values := make([]float64, 0, len(fields))
		for _, f := range fields {
			v, err := strconv.ParseFloat(f, 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				break
			}
			values = append(values, v)
		}
		if len(values) < 2 || len(values) != len(fields) {
			issues = append(issues, fmt.Sprintf("line %d: unrecognized measurement row", line))
			continue
		}

		p := Point{Line: line, Frequency: values[0], SPL: values[1], Values: values}
		if len(values) > 2 {
			p.Phase = &values[2]
		}
		points = append(points, p)
	}

	if err := scanner.Err(); err != nil {
		issues = append(issues, err.Error())
	}

	if len(points) == 0 {
		issues = append(issues, "no numeric measurement points")
	}

	return points, issues
}

type layout struct {
	Directory string
	Channels  []string
	Samples   int
}

var configSetting = regexp.MustCompile(`(?m)\b(DIR|default_channels|num_samples)\s*=\s*("[^"\r\n]*"|'[^'\r\n]*'|\[[^\]\r\n]*\]|[0-9]+)`)
var jsComment = regexp.MustCompile(`(?m)/\*[\s\S]*?\*/|(^|\s)//[^\r\n]*`)

func parseLayout(data []byte) (layout, error) {
	l := layout{Directory: "data/", Channels: []string{"L", "R"}}
	clean := jsComment.ReplaceAllString(string(data), "$1")

	for _, m := range configSetting.FindAllStringSubmatch(clean, -1) {
		switch m[1] {
		case "DIR":
			l.Directory = m[2][1 : len(m[2])-1]
		case "default_channels":
			if err := json.Unmarshal([]byte(strings.ReplaceAll(m[2], "'", `"`)), &l.Channels); err != nil {
				return l, err
			}
		case "num_samples":
			l.Samples, _ = strconv.Atoi(m[2])
			if l.Samples < 1 || l.Samples > 1000 {
				return l, fmt.Errorf("unsupported num_samples %d", l.Samples)
			}
		}
	}

	if len(l.Channels) == 0 {
		return l, fmt.Errorf("empty default_channels")
	}

	return l, nil
}
