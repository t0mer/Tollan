package functions

import (
	"encoding/csv"
	"encoding/json"
	"strconv"
	"strings"
)

// ParseKV parses key=value pairs from s. Values may be bare or double-quoted.
// Pairs are separated by whitespace. Returns the extracted map.
func ParseKV(s string) map[string]string {
	out := map[string]string{}
	i := 0
	for i < len(s) {
		// skip separators
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == ',' || s[i] == ';') {
			i++
		}
		// read key
		start := i
		for i < len(s) && s[i] != '=' && s[i] != ' ' {
			i++
		}
		if i >= len(s) || s[i] != '=' {
			// no '=', skip token
			for i < len(s) && s[i] != ' ' {
				i++
			}
			continue
		}
		key := s[start:i]
		i++ // skip '='
		var val string
		if i < len(s) && s[i] == '"' {
			i++
			vs := i
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' && i+1 < len(s) {
					i++
				}
				i++
			}
			val = strings.ReplaceAll(s[vs:i], `\"`, `"`)
			if i < len(s) {
				i++ // closing quote
			}
		} else {
			vs := i
			for i < len(s) && s[i] != ' ' && s[i] != '\t' {
				i++
			}
			val = s[vs:i]
		}
		if key != "" {
			out[key] = val
		}
	}
	return out
}

// ParseCSV parses a single CSV record from s and maps its columns to the given
// column names. Extra columns are dropped; missing columns are skipped.
func ParseCSV(s string, columns []string) (map[string]string, error) {
	r := csv.NewReader(strings.NewReader(s))
	r.FieldsPerRecord = -1
	rec, err := r.Read()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for i, col := range columns {
		if i < len(rec) && col != "" {
			out[col] = rec[i]
		}
	}
	return out, nil
}

// ParseJSON parses s as a JSON object into a flat map of top-level keys.
func ParseJSON(s string) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	return m, nil
}

// Coerce converts a string value to the named type: int, float, bool, string.
func Coerce(value, typ string) (any, bool) {
	switch typ {
	case "int":
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, false
		}
		return n, true
	case "float":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, false
		}
		return f, true
	case "bool":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return nil, false
		}
		return b, true
	case "string":
		return value, true
	default:
		return nil, false
	}
}
