// Package query implements Tollan's Lucene-style search query language: a
// tokenizer, a recursive-descent parser producing an AST, and (in the store) a
// compiler from that AST to SQL over the log partitions. Grammar (precedence
// low→high): OR, implicit/explicit AND, NOT, primary. Supported leaves:
//
//	term                full-text match on the message body ("foo", "foo*", "a phrase")
//	field:value         equality (wildcards * and ? allowed)
//	field:>N            numeric comparison (> < >= <=)
//	field:[a TO b]      inclusive range   field:{a TO b} exclusive
//	_exists_:field      field presence
package query

// Node is a query AST node.
type Node interface{ isNode() }

// MatchAll matches every message (produced by an empty query).
type MatchAll struct{}

// And is a conjunction.
type And struct{ Left, Right Node }

// Or is a disjunction.
type Or struct{ Left, Right Node }

// Not negates its child.
type Not struct{ Expr Node }

// Term is a full-text match on the message body.
type Term struct {
	Value  string
	Phrase bool // came from a quoted string
}

// FieldEq is field:value equality; Value may contain wildcards (* and ?).
type FieldEq struct {
	Field string
	Value string
}

// FieldExists is _exists_:field.
type FieldExists struct{ Field string }

// Comparison operators for FieldCompare.
const (
	OpGT  = ">"
	OpLT  = "<"
	OpGTE = ">="
	OpLTE = "<="
)

// FieldCompare is a numeric comparison, e.g. status:>=400.
type FieldCompare struct {
	Field string
	Op    string
	Value string
}

// FieldRange is field:[lo TO hi] (inclusive) or {lo TO hi} (exclusive). A bound
// of "*" is open-ended.
type FieldRange struct {
	Field     string
	Lo, Hi    string
	IncludeLo bool
	IncludeHi bool
}

func (MatchAll) isNode()      {}
func (*And) isNode()          {}
func (*Or) isNode()           {}
func (*Not) isNode()          {}
func (*Term) isNode()         {}
func (*FieldEq) isNode()      {}
func (*FieldExists) isNode()  {}
func (*FieldCompare) isNode() {}
func (*FieldRange) isNode()   {}

// HasWildcard reports whether s contains an unescaped wildcard (* or ?).
func HasWildcard(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '*' || s[i] == '?' {
			return true
		}
	}
	return false
}
