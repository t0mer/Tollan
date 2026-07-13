package query

import (
	"fmt"
	"testing"
)

// dump renders an AST to a canonical parenthesized string for assertions.
func dump(n Node) string {
	switch t := n.(type) {
	case MatchAll:
		return "ALL"
	case *And:
		return fmt.Sprintf("(%s AND %s)", dump(t.Left), dump(t.Right))
	case *Or:
		return fmt.Sprintf("(%s OR %s)", dump(t.Left), dump(t.Right))
	case *Not:
		return fmt.Sprintf("NOT %s", dump(t.Expr))
	case *Term:
		if t.Phrase {
			return fmt.Sprintf("phrase(%q)", t.Value)
		}
		return fmt.Sprintf("term(%s)", t.Value)
	case *FieldEq:
		return fmt.Sprintf("%s=%s", t.Field, t.Value)
	case *FieldExists:
		return fmt.Sprintf("exists(%s)", t.Field)
	case *FieldCompare:
		return fmt.Sprintf("%s%s%s", t.Field, t.Op, t.Value)
	case *FieldRange:
		lo, hi := "[", "]"
		if !t.IncludeLo {
			lo = "{"
		}
		if !t.IncludeHi {
			hi = "}"
		}
		return fmt.Sprintf("%s:%s%s TO %s%s", t.Field, lo, t.Lo, t.Hi, hi)
	default:
		return "?"
	}
}

func TestParseValid(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "ALL"},
		{"   ", "ALL"},
		{"error", "term(error)"},
		{"error*", "term(error*)"},
		{`"disk full"`, `phrase("disk full")`},
		{"foo bar", "(term(foo) AND term(bar))"},
		{"foo AND bar", "(term(foo) AND term(bar))"},
		{"foo OR bar", "(term(foo) OR term(bar))"},
		{"NOT foo", "NOT term(foo)"},
		{"foo NOT bar", "(term(foo) AND NOT term(bar))"},
		{"level:error", "level=error"},
		{"source:web01 level:error", "(source=web01 AND level=error)"},
		{"status:>=400", "status>=400"},
		{"status:>400", "status>400"},
		{"bytes:<100", "bytes<100"},
		{"status:[400 TO 599]", "status:[400 TO 599]"},
		{"status:{400 TO 599}", "status:{400 TO 599}"},
		{"ts:[* TO 5]", "ts:[* TO 5]"},
		{"_exists_:src_ip", "exists(src_ip)"},
		{"a OR b AND c", "(term(a) OR (term(b) AND term(c)))"},
		{"(a OR b) AND c", "((term(a) OR term(b)) AND term(c))"},
		{"NOT (a OR b)", "NOT (term(a) OR term(b))"},
		{`msg:"not found"`, `msg=not found`},
		{"level:error OR level:critical", "(level=error OR level=critical)"},
		{"error AND NOT level:debug", "(term(error) AND NOT level=debug)"},
		{"src_ip:10.0.0.0/8", "src_ip=10.0.0.0/8"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			n, err := Parse(c.in)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", c.in, err)
			}
			if got := dump(n); got != c.want {
				t.Errorf("Parse(%q) = %s, want %s", c.in, got, c.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	cases := []string{
		`"unterminated`,
		"(unbalanced",
		"level:",
		"level:>",
		"status:[400 TO]",
		"status:[400 599]", // missing TO
		")",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if _, err := Parse(c); err == nil {
				t.Errorf("Parse(%q) = nil error, want error", c)
			}
		})
	}
}

func TestPrecedence(t *testing.T) {
	// AND binds tighter than OR; NOT tighter than AND.
	n, err := Parse("a AND b OR c AND d")
	if err != nil {
		t.Fatal(err)
	}
	if got := dump(n); got != "((term(a) AND term(b)) OR (term(c) AND term(d)))" {
		t.Errorf("precedence = %s", got)
	}
}
