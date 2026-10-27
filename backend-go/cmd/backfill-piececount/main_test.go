package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The real pipeline artifact is UTF-8 with BOM and CRLF. A hand-written demo CSV
// without those bytes is not a test of the parser (lesson from import-longdesc).
func TestReadCSVRealWorldBytes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pc.csv")
	body := "\ufeffid,piece_count\r\n" +
		"6e26985b-98be-4e82-b661-d404fa6eee21,171\r\n" +
		"22e5e983325a4b108646b4ea4538e6da,3420\r\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := readCSV(p)
	if err != nil {
		t.Fatalf("readCSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	if rows[0].ID != "6e26985b-98be-4e82-b661-d404fa6eee21" || rows[0].Pieces != 171 {
		t.Fatalf("row 0 = %+v", rows[0])
	}
	// 32-char id without dashes exists in production and must not be normalised.
	if rows[1].ID != "22e5e983325a4b108646b4ea4538e6da" || rows[1].Pieces != 3420 {
		t.Fatalf("row 1 = %+v", rows[1])
	}
}

func TestReadCSVRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"zero":     "\ufeffid,piece_count\r\nabc-123,0\r\n",
		"negative": "\ufeffid,piece_count\r\nabc-123,-5\r\n",
		"not int":  "\ufeffid,piece_count\r\nabc-123,twelve\r\n",
		"dup":      "\ufeffid,piece_count\r\naaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa,10\r\naaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa,11\r\n",
		"bad id":   "\ufeffid,piece_count\r\nshort,10\r\n",
		"too few":  "\ufeffid,piece_count\r\naaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\r\n",
		"no data":  "\ufeffid,piece_count\r\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "bad.csv")
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := readCSV(p); err == nil {
				t.Fatalf("%s: want error, got none", name)
			}
		})
	}
}

// classify() is where the "do not overwrite an existing count" rule lives on the
// read side; the write side enforces it again in SQL. Both must agree.
func TestClassify(t *testing.T) {
	zero := int64(0)
	has := int64(500)
	same := int64(171)
	cur := map[string]*current{
		"zero": {ID: "zero", Pieces: &zero},
		"nul":  {ID: "nul", Pieces: nil},
		"has":  {ID: "has", Pieces: &has},
		"same": {ID: "same", Pieces: &same},
	}
	rows := []row{
		{ID: "zero", Pieces: 171},
		{ID: "nul", Pieces: 3420},
		{ID: "has", Pieces: 999},  // csv disagrees with a stored value -> protected
		{ID: "same", Pieces: 171}, // identical -> unchanged
		{ID: "gone", Pieces: 12},  // not in db -> missing
	}
	p := classify(rows, cur)
	if len(p.update) != 2 {
		t.Fatalf("update = %+v, want 2 (zero + NULL)", p.update)
	}
	if len(p.protected) != 1 || p.protected[0].r.ID != "has" || p.protected[0].cur != 500 {
		t.Fatalf("protected = %+v", p.protected)
	}
	if len(p.same) != 1 || p.same[0].ID != "same" {
		t.Fatalf("same = %+v", p.same)
	}
	if len(p.missing) != 1 || p.missing[0] != "gone" {
		t.Fatalf("missing = %+v", p.missing)
	}
}

// The WHERE clause is the contract with the ticket: it must protect a non-zero
// value and must not depend on the caller passing a "force" flag.
func TestUpdateSQLGuard(t *testing.T) {
	if !strings.Contains(updateSQL, "AND (piece_count IS NULL OR piece_count = 0)") {
		t.Fatalf("guard clause missing from: %s", updateSQL)
	}
	if strings.Contains(updateSQL, "description") {
		t.Fatalf("tool must only touch piece_count: %s", updateSQL)
	}
}

func TestLoadProgressDryRunIsInert(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.progress")
	if err := os.WriteFile(p, []byte("abc\t2026-01-01T00:00:00Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadProgress(p, true); len(got) != 0 {
		t.Fatalf("dry-run must ignore the ledger, got %v", got)
	}
	if got := loadProgress(p, false); !got["abc"] {
		t.Fatalf("apply must honour the ledger, got %v", got)
	}
}
