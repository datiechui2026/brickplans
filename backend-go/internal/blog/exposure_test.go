package blog

import (
	"os"
	"path/filepath"
	"testing"
)

func writeLedger(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write ledger: %v", err)
	}
}

func TestExposureLedgerMissingFileAdvertisesEverything(t *testing.T) {
	var nilLedger *ExposureLedger
	if !nilLedger.Allows("anything") {
		t.Fatal("nil ledger must advertise everything")
	}
	if nilLedger.Active() || nilLedger.Len() != 0 {
		t.Fatal("nil ledger must be inactive")
	}

	l := NewExposureLedger(filepath.Join(t.TempDir(), "nope.txt"))
	if !l.Allows("anything") {
		t.Fatal("missing ledger file must advertise everything")
	}
	if l.Active() {
		t.Fatal("missing ledger file must report inactive")
	}

	empty := NewExposureLedger("")
	if !empty.Allows("anything") {
		t.Fatal("empty path must advertise everything")
	}
}

func TestExposureLedgerParsesSlugs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sitemap-exposure.txt")
	writeLedger(t, path, "\ufeff# released 2026-10-24\n\nfirst-slug\n  second-slug  \r\n#third\n")

	l := NewExposureLedger(path)
	if !l.Active() {
		t.Fatal("ledger with slugs must be active")
	}
	if got := l.Len(); got != 2 {
		t.Fatalf("Len = %d, want 2 (comments/blanks/CR must be ignored)", got)
	}
	for _, slug := range []string{"first-slug", "second-slug"} {
		if !l.Allows(slug) {
			t.Fatalf("%q must be exposed", slug)
		}
	}
	for _, slug := range []string{"third", "not-listed", ""} {
		if l.Allows(slug) {
			t.Fatalf("%q must not be exposed", slug)
		}
	}
}

func TestExposureLedgerReloadsOnChangeWithoutRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sitemap-exposure.txt")
	writeLedger(t, path, "a\n")

	l := NewExposureLedger(path)
	if !l.Allows("a") || l.Allows("b") {
		t.Fatal("initial load wrong")
	}

	// Appending changes both size and mtime — the production append path.
	writeLedger(t, path, "a\nb\n")
	if !l.Allows("b") {
		t.Fatal("appended slug must be visible without recreating the ledger")
	}

	// Deleting restores "advertise everything".
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !l.Allows("c") || l.Active() {
		t.Fatal("deleted ledger must fall back to advertising everything")
	}
}
