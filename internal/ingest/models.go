package ingest

import (
	"encoding/json"
	"strconv"
)

type SquigSite struct {
	Name      string    `json:"name"`
	Username  string    `json:"username"`
	URLType   string    `json:"urlType"`
	AltDomain string    `json:"altDomain,omitempty"`
	DBs       []SquigDB `json:"dbs"`
}

type SquigDB struct {
	Type       string       `json:"type"`
	Folder     string       `json:"folder"`
	DeltaReady FlexibleBool `json:"deltaReady,omitempty"`
}

type FlexibleBool bool

func (b *FlexibleBool) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		v, err := strconv.ParseBool(s)
		*b = FlexibleBool(v)
		return err
	}
	var v bool
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*b = FlexibleBool(v)
	return nil
}

type Brand struct {
	Name   string  `json:"name"`
	Phones []Phone `json:"phones"`
}

type Phone struct {
	Name        string      `json:"name"`
	ReviewScore string      `json:"reviewScore"`
	ReviewLink  string      `json:"reviewLink"`
	Price       string      `json:"price"`
	ShopLink    string      `json:"shopLink"`
	Files       StringSlice `json:"file"`
	Suffixes    StringSlice `json:"suffix"`
}

type StringSlice []string

func (s *StringSlice) UnmarshalJSON(b []byte) error {
	var single string
	if err := json.Unmarshal(b, &single); err == nil {
		*s = []string{single}
		return nil
	}
	var multi []string
	if err := json.Unmarshal(b, &multi); err != nil {
		return err
	}
	*s = multi
	return nil
}
