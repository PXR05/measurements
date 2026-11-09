package shared

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
)

type ListOptions struct {
	Limit, Offset int
	Sort, Order   string
}

func ParseListOptions(
	values url.Values,
	sorts map[string]string,
	defaultSort string,
	defaultLimit, maxLimit int,
) (ListOptions, error) {
	o := ListOptions{Limit: defaultLimit, Sort: sorts[defaultSort], Order: "ASC"}

	var err error
	o.Limit, err = IntegerValue(values, "limit", defaultLimit)
	if err != nil {
		return o, err
	}
	o.Offset, err = IntegerValue(values, "offset", 0)
	if err != nil {
		return o, err
	}

	if o.Limit < 1 || o.Limit > maxLimit {
		return o, Invalid("limit must be between 1 and %d", maxLimit)
	}

	if o.Offset > 10_000_000 {
		return o, Invalid("offset must not exceed 10000000")
	}

	if values.Has("sort") {
		value, err := singleValue(values, "sort")
		if err != nil {
			return o, err
		}

		var ok bool
		o.Sort, ok = sorts[value]
		if !ok {
			return o, Invalid("unsupported sort %q", value)
		}
	}

	if values.Has("order") {
		value, err := singleValue(values, "order")
		if err != nil {
			return o, err
		}

		o.Order = strings.ToUpper(value)
		if o.Order != "ASC" && o.Order != "DESC" {
			return o, Invalid("order must be asc or desc")
		}
	}

	return o, nil
}

func IntegerValue(values url.Values, key string, fallback int) (int, error) {
	if !values.Has(key) {
		return fallback, nil
	}

	value, err := singleValue(values, key)
	if err != nil {
		return 0, err
	}

	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, Invalid("%s must be a nonnegative integer", key)
	}

	return n, nil
}

func singleValue(values url.Values, key string) (string, error) {
	if len(values[key]) != 1 {
		return "", Invalid("%s must be provided once", key)
	}

	return values.Get(key), nil
}

func BoolValue(values url.Values, key string) (bool, error) {
	if !values.Has(key) {
		return false, nil
	}

	value, err := singleValue(values, key)
	if err != nil {
		return false, err
	}

	if value != "true" && value != "false" {
		return false, Invalid("%s must be true or false", key)
	}

	return value == "true", nil
}

func PositiveID(value, name string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		return 0, Invalid("%s must be a positive integer", name)
	}

	return id, nil
}

func ValidateKeys(values url.Values, allowed ...string) error {
	return ValidateOnlyKeys(values, append([]string{"limit", "offset", "sort", "order"}, allowed...)...)
}

func ValidateOnlyKeys(values url.Values, allowed ...string) error {
	keys := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		keys[key] = true
	}
	for key := range values {
		if !keys[key] {
			return Invalid("unknown query parameter %q", key)
		}
	}
	return nil
}

func SearchText(values url.Values) (string, error) {
	if !values.Has("q") {
		return "", nil
	}
	text, err := singleValue(values, "q")
	if err != nil {
		return "", err
	}
	if len(text) > 500 {
		return "", Invalid("q must not exceed 500 bytes")
	}
	return text, nil
}

func StringSelection(values url.Values, key string, normalize bool) (string, error) {
	inputs := values[key]
	if len(inputs) > 50 {
		return "", Invalid("too many values for %s", key)
	}
	items := make([]string, 0, len(inputs))
	for _, value := range inputs {
		if normalize {
			value = NormalizeName(value)
		}
		if value == "" && key != "sample" {
			return "", Invalid("%s must not be empty", key)
		}
		items = append(items, value)
	}
	data, _ := json.Marshal(items)
	return string(data), nil
}

func FiniteNumber(values url.Values, key string) (float64, error) {
	value, err := singleValue(values, key)
	if err != nil {
		return 0, err
	}

	n, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsInf(n, 0) || math.IsNaN(n) || n < 0 {
		return 0, Invalid("%s must be a finite nonnegative number", key)
	}

	return n, nil
}

type RequestError struct {
	Status  int
	Message string
}

func (e *RequestError) Error() string { return e.Message }
func Invalid(format string, args ...any) error {
	return &RequestError{Status: 400, Message: fmt.Sprintf(format, args...)}
}

func LowerOrder(order string) string {
	if order == "DESC" {
		return "desc"
	}
	return "asc"
}

func BoolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func NormalizeName(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, strings.Trim(name, " \t\r\n"))
}
