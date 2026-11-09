package iems

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"measurements/internal/storage/dbgen"
)

type sourceConfiguration struct {
	rigDescription  *string
	calibrationFile *string
}

func loadConfigurations(ctx context.Context, queries *dbgen.Queries, rows []dbgen.ListMeasurementsRow) (map[int64]sourceConfiguration, error) {
	ids := make([]int64, 0)
	seen := make(map[int64]bool)
	for _, row := range rows {
		if row.ConfigResourceID.Valid && !seen[row.ConfigResourceID.Int64] {
			ids = append(ids, row.ConfigResourceID.Int64)
			seen[row.ConfigResourceID.Int64] = true
		}
	}
	result := make(map[int64]sourceConfiguration)
	if len(ids) == 0 {
		return result, nil
	}
	data, _ := json.Marshal(ids)
	resources, err := queries.ListConfigResources(ctx, string(data))
	if err != nil {
		return nil, err
	}
	for _, resource := range resources {
		result[resource.ID] = parseSourceConfiguration(string(resource.Body))
	}
	return result, nil
}

func parseSourceConfiguration(text string) sourceConfiguration {
	var config sourceConfiguration
	for i := 0; i < len(text); {
		i = skipConfigTrivia(text, i)
		if i >= len(text) {
			break
		}
		if text[i] == '\'' || text[i] == '"' || text[i] == '`' {
			_, i, _ = readConfigString(text, i)
			continue
		}
		if !configIdentifier(text[i]) {
			i++
			continue
		}
		start := i
		for i < len(text) && configIdentifier(text[i]) {
			i++
		}
		key := text[start:i]
		if key != "rig_description" && key != "measurement_calibration_file" {
			continue
		}
		position := skipConfigTrivia(text, i)
		if position >= len(text) || text[position] != '=' {
			continue
		}
		position = skipConfigTrivia(text, position+1)
		value, end, ok := readConfigString(text, position)
		if !ok {
			continue
		}
		if next := skipConfigTrivia(text, end); next < len(text) && !strings.ContainsRune(",;}", rune(text[next])) && !strings.ContainsAny(text[end:next], "\r\n") {
			continue
		}
		i = end
		var setting *string
		if value != "" {
			setting = &value
		}
		if key == "rig_description" {
			config.rigDescription = setting
		} else {
			config.calibrationFile = setting
		}
	}
	return config
}

func configIdentifier(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '$'
}

func skipConfigTrivia(text string, i int) int {
	for i < len(text) {
		switch {
		case strings.ContainsRune(" \t\r\n", rune(text[i])):
			i++
		case strings.HasPrefix(text[i:], "//"):
			end := strings.IndexByte(text[i:], '\n')
			if end < 0 {
				return len(text)
			}
			i += end + 1
		case strings.HasPrefix(text[i:], "/*"):
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				return len(text)
			}
			i += end + 4
		default:
			return i
		}
	}
	return i
}

func readConfigString(text string, start int) (string, int, bool) {
	if start >= len(text) || text[start] != '\'' && text[start] != '"' && text[start] != '`' {
		return "", start, false
	}
	quote := text[start]
	end := start + 1
	for end < len(text) && text[end] != quote {
		if text[end] == '\\' {
			end++
		}
		end++
	}
	if end >= len(text) {
		return "", len(text), false
	}
	if quote == '`' {
		return "", end + 1, false
	}
	if quote == '"' {
		var value string
		if json.Unmarshal([]byte(text[start:end+1]), &value) == nil {
			return value, end + 1, true
		}
	}
	remaining := text[start+1 : end]
	var value strings.Builder
	for remaining != "" {
		if strings.HasPrefix(remaining, `\/`) {
			value.WriteByte('/')
			remaining = remaining[2:]
			continue
		}
		character, _, tail, err := strconv.UnquoteChar(remaining, quote)
		if err != nil {
			return "", end + 1, false
		}
		value.WriteRune(character)
		remaining = tail
	}
	return value.String(), end + 1, true
}
