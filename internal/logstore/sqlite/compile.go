package sqlite

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/t0mer/tollan/internal/search/query"
)

// compileExpr turns a query AST into a SQL boolean expression over the messages
// table plus its bound arguments. Full-text leaves become rowid-IN subqueries
// against the FTS index so they compose freely with AND/OR/NOT and field
// predicates. Field names are strictly validated because they are interpolated
// into JSON paths; all values are parameterized.
func compileExpr(n query.Node) (string, []any, error) {
	c := &compiler{}
	sql, err := c.walk(n)
	if err != nil {
		return "", nil, err
	}
	return sql, c.args, nil
}

type compiler struct {
	args []any
}

func (c *compiler) add(v any) string {
	c.args = append(c.args, v)
	return "?"
}

func (c *compiler) walk(n query.Node) (string, error) {
	switch t := n.(type) {
	case query.MatchAll:
		return "1=1", nil
	case *query.And:
		return c.binary(t.Left, t.Right, "AND")
	case *query.Or:
		return c.binary(t.Left, t.Right, "OR")
	case *query.Not:
		e, err := c.walk(t.Expr)
		if err != nil {
			return "", err
		}
		return "(NOT " + e + ")", nil
	case *query.Term:
		return c.fullText(t.Value, t.Phrase), nil
	case *query.FieldEq:
		return c.fieldEq(t.Field, t.Value)
	case *query.FieldExists:
		return c.exists(t.Field)
	case *query.FieldCompare:
		return c.compare(t.Field, t.Op, t.Value)
	case *query.FieldRange:
		return c.rangeExpr(t)
	default:
		return "", fmt.Errorf("unsupported query node %T", n)
	}
}

func (c *compiler) binary(l, r query.Node, op string) (string, error) {
	ls, err := c.walk(l)
	if err != nil {
		return "", err
	}
	rs, err := c.walk(r)
	if err != nil {
		return "", err
	}
	return "(" + ls + " " + op + " " + rs + ")", nil
}

// fullText emits a rowid-IN subquery against the FTS index.
func (c *compiler) fullText(value string, phrase bool) string {
	match := ftsMatch(value, phrase)
	return "m.rowid IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH " + c.add(match) + ")"
}

func (c *compiler) fieldEq(field, value string) (string, error) {
	if isFullText(field) {
		return c.fullText(value, false), nil
	}
	col, ok := columnExpr(field)
	if !ok {
		if err := validateFieldName(field); err != nil {
			return "", err
		}
		col = jsonExpr(field)
	}
	if query.HasWildcard(value) {
		return "CAST(" + col + " AS TEXT) LIKE " + c.add(likePattern(value)) + " ESCAPE '\\'", nil
	}
	return "CAST(" + col + " AS TEXT) = " + c.add(value), nil
}

func (c *compiler) exists(field string) (string, error) {
	if col, ok := columnExpr(field); ok {
		return "(" + col + " IS NOT NULL AND " + col + " != '')", nil
	}
	if err := validateFieldName(field); err != nil {
		return "", err
	}
	return jsonExpr(field) + " IS NOT NULL", nil
}

func (c *compiler) compare(field, op, value string) (string, error) {
	num, isNum := c.numericExpr(field)
	if !validOp(op) {
		return "", fmt.Errorf("invalid comparison operator %q", op)
	}
	if isNum {
		v, err := c.numericValue(field, value)
		if err != nil {
			return "", err
		}
		return num + " " + op + " " + c.add(v), nil
	}
	// Non-numeric field: lexical comparison.
	col, err := c.textExpr(field)
	if err != nil {
		return "", err
	}
	return "CAST(" + col + " AS TEXT) " + op + " " + c.add(value), nil
}

func (c *compiler) rangeExpr(r *query.FieldRange) (string, error) {
	num, isNum := c.numericExpr(r.Field)
	var col string
	if !isNum {
		var err error
		if col, err = c.textExpr(r.Field); err != nil {
			return "", err
		}
		col = "CAST(" + col + " AS TEXT)"
	} else {
		col = num
	}

	var parts []string
	if r.Lo != "*" {
		op := ">="
		if !r.IncludeLo {
			op = ">"
		}
		parts = append(parts, col+" "+op+" "+c.add(c.rangeBound(r.Field, r.Lo, isNum)))
	}
	if r.Hi != "*" {
		op := "<="
		if !r.IncludeHi {
			op = "<"
		}
		parts = append(parts, col+" "+op+" "+c.add(c.rangeBound(r.Field, r.Hi, isNum)))
	}
	if len(parts) == 0 {
		return "1=1", nil
	}
	return "(" + strings.Join(parts, " AND ") + ")", nil
}

func (c *compiler) rangeBound(field, v string, numeric bool) any {
	if !numeric {
		return v
	}
	n, err := c.numericValue(field, v)
	if err != nil {
		return v
	}
	return n
}

// numericExpr returns the numeric SQL expression for a field and whether the
// field is treated numerically (timestamps and the received column).
func (c *compiler) numericExpr(field string) (string, bool) {
	switch field {
	case "timestamp", "ts":
		return "m.ts", true
	case "received", "received_at":
		return "m.received", true
	}
	if _, ok := columnExpr(field); ok {
		return "", false // text columns compare lexically
	}
	if validateFieldName(field) == nil {
		return "CAST(" + jsonExpr(field) + " AS REAL)", true
	}
	return "", false
}

func (c *compiler) textExpr(field string) (string, error) {
	if col, ok := columnExpr(field); ok {
		return col, nil
	}
	if err := validateFieldName(field); err != nil {
		return "", err
	}
	return jsonExpr(field), nil
}

// numericValue parses a comparison/range value, converting timestamp fields from
// RFC3339 to epoch millis.
func (c *compiler) numericValue(field, value string) (float64, error) {
	if field == "timestamp" || field == "ts" || field == "received" || field == "received_at" {
		if t, err := time.Parse(time.RFC3339, value); err == nil {
			return float64(t.UTC().UnixMilli()), nil
		}
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("value %q is not numeric", value)
	}
	return n, nil
}

// --- field mapping & helpers ---

func isFullText(field string) bool {
	return field == "message" || field == "body"
}

// columnExpr maps a field name to a real message column, if any.
func columnExpr(field string) (string, bool) {
	switch field {
	case "source":
		return "m.source", true
	case "stream":
		return "m.stream", true
	case "input_id", "input":
		return "m.input_id", true
	case "id":
		return "m.id", true
	default:
		return "", false
	}
}

func jsonExpr(field string) string {
	// field is validated to [A-Za-z0-9_.-]+, safe to interpolate as a JSON key.
	return "json_extract(m.fields, '$.\"" + field + "\"')"
}

func validOp(op string) bool {
	switch op {
	case query.OpGT, query.OpLT, query.OpGTE, query.OpLTE:
		return true
	}
	return false
}

// validateFieldName restricts field names to a safe character set so they can be
// interpolated into a JSON path without SQL injection risk.
func validateFieldName(field string) error {
	if field == "" {
		return fmt.Errorf("empty field name")
	}
	for i := 0; i < len(field); i++ {
		c := field[i]
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '.' || c == '-'
		if !ok {
			return fmt.Errorf("invalid character %q in field name %q", string(c), field)
		}
	}
	return nil
}

// ftsMatch builds an FTS5 MATCH expression for a full-text value.
func ftsMatch(value string, phrase bool) string {
	if phrase {
		return `"` + strings.ReplaceAll(value, `"`, " ") + `"`
	}
	// Prefix search: a single trailing '*' becomes an FTS prefix token.
	if strings.HasSuffix(value, "*") && strings.Count(value, "*") == 1 {
		stem := sanitizeFTSToken(strings.TrimSuffix(value, "*"))
		if stem != "" {
			return stem + "*"
		}
	}
	// Otherwise match the value as a literal phrase to avoid interpreting FTS
	// operators embedded in the term.
	return `"` + strings.ReplaceAll(value, `"`, " ") + `"`
}

func sanitizeFTSToken(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// likePattern translates wildcard syntax (* ?) to SQL LIKE (% _), escaping any
// literal %, _ and backslash with a backslash escape.
func likePattern(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '*':
			b.WriteByte('%')
		case '?':
			b.WriteByte('_')
		case '%', '_', '\\':
			b.WriteByte('\\')
			b.WriteByte(value[i])
		default:
			b.WriteByte(value[i])
		}
	}
	return b.String()
}
