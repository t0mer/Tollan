package functions

import "testing"

func TestGrokSSHD(t *testing.T) {
	g := NewGrok()
	c, err := g.Compile("%{SSHDFAIL}")
	if err != nil {
		t.Fatal(err)
	}
	m, ok := c.Match("Failed password for invalid user admin from 198.51.100.9")
	if !ok {
		t.Fatal("no match")
	}
	if m["user"] != "admin" || m["src_ip"] != "198.51.100.9" {
		t.Errorf("got %v", m)
	}
}

func TestGrokCustomFields(t *testing.T) {
	g := NewGrok()
	c, err := g.Compile("%{WORD:method} %{URIPATH:path} %{INT:status}")
	if err != nil {
		t.Fatal(err)
	}
	m, ok := c.Match("GET /orders 500")
	if !ok {
		t.Fatal("no match")
	}
	if m["method"] != "GET" || m["path"] != "/orders" || m["status"] != "500" {
		t.Errorf("got %v", m)
	}
}

func TestGrokUnknownPattern(t *testing.T) {
	if _, err := NewGrok().Compile("%{NOPE:x}"); err == nil {
		t.Fatal("expected error for unknown pattern")
	}
}

func TestParseKV(t *testing.T) {
	m := ParseKV(`src=10.0.0.1 dst=10.0.0.2 action="drop packet" proto=TCP`)
	if m["src"] != "10.0.0.1" || m["dst"] != "10.0.0.2" || m["action"] != "drop packet" || m["proto"] != "TCP" {
		t.Errorf("got %v", m)
	}
}

func TestParseCSV(t *testing.T) {
	m, err := ParseCSV(`alice,42,admin`, []string{"user", "id", "role"})
	if err != nil {
		t.Fatal(err)
	}
	if m["user"] != "alice" || m["id"] != "42" || m["role"] != "admin" {
		t.Errorf("got %v", m)
	}
}

func TestCoerce(t *testing.T) {
	if v, ok := Coerce("42", "int"); !ok || v.(int64) != 42 {
		t.Errorf("int coerce = %v %v", v, ok)
	}
	if v, ok := Coerce("3.14", "float"); !ok || v.(float64) != 3.14 {
		t.Errorf("float coerce = %v %v", v, ok)
	}
	if _, ok := Coerce("x", "int"); ok {
		t.Error("expected int coerce to fail")
	}
}
