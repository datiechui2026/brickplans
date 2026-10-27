package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The real payload (/root/team/seo/_data/samples/desc_full_clean.csv) is
// utf-8-sig with CRLF record separators and LF inside quoted bodies. A BOM left
// on the header cell turns every id into "\ufeffid"-style garbage and the whole
// run reports "missing ids: 2133", so this file pins the parsing contract.
func writeTempCSV(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "in.csv")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write temp csv: %v", err)
	}
	return p
}

func TestReadCSVBOMAndCRLF(t *testing.T) {
	body := "\ufeffid,zh,en\r\n" +
		"1995e800-9882-4826-b8e1-9d1e2f6c2d89,\"第一段\n\n第二段\",\"First part.\n\nSecond part.\"\r\n" +
		"0eb78ed4365743bda74b2202bf13da5c,纯中文,English only\r\n"
	rows, err := readCSV(writeTempCSV(t, body))
	if err != nil {
		t.Fatalf("readCSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (header must be skipped)", len(rows))
	}
	if rows[0].ID != "1995e800-9882-4826-b8e1-9d1e2f6c2d89" {
		t.Errorf("id = %q, want the BOM-free header-skipped value", rows[0].ID)
	}
	if rows[0].ZH != "第一段\n\n第二段" {
		t.Errorf("zh = %q, want the quoted multi-line body intact", rows[0].ZH)
	}
	if rows[1].ID != "0eb78ed4365743bda74b2202bf13da5c" {
		t.Errorf("dashless id = %q, want it kept verbatim (production really has one)", rows[1].ID)
	}
	if rows[0].Ln != 2 || rows[1].Ln != 3 {
		t.Errorf("line numbers = %d,%d, want 2,3", rows[0].Ln, rows[1].Ln)
	}
}

func TestReadCSVRejectsMissingColumnAndDup(t *testing.T) {
	if _, err := readCSV(writeTempCSV(t, "id,zh\nabcdefghijabcdefghijabcdefghijab,只有中\n")); err == nil {
		t.Error("2-column row must be rejected, got nil error")
	}
	dup := "id,zh,en\n" +
		"abcdefghijabcdefghijabcdefghijab,一,x\n" +
		"abcdefghijabcdefghijabcdefghijab,二,y\n"
	if _, err := readCSV(writeTempCSV(t, dup)); err == nil {
		t.Error("duplicate id must be rejected, got nil error")
	}
}

// EN length is prose in latin script: rune bounds would flag every row, word
// budget is the meaningful unit (production mean is 163 words).
func TestWordCount(t *testing.T) {
	cases := map[string]int{
		"":                             0,
		"   \n\t ":                     0,
		"one":                          1,
		"Colours stay low-saturation.": 3,
		"a\n\nb   c\td":                4,
	}
	for in, want := range cases {
		if got := wordCount(in); got != want {
			t.Errorf("wordCount(%q) = %d, want %d", in, got, want)
		}
	}
}
