// Command backfill-piececount fills blueprints.piece_count from a 2-column CSV
// (id,piece_count) and NEVER overwrites a value that is already set.
//
//	backfill-piececount --csv pc.csv --dry-run
//	backfill-piececount --csv pc.csv --apply
//
// Contract with the ticket (SEO ticket 02, batch 2 follow-up):
//
//   - CSV is id,piece_count. Header optional, BOM and CRLF tolerated.
//   - The only statement this tool can emit is
//     UPDATE blueprints SET piece_count = ? WHERE id = ? AND (piece_count IS NULL OR piece_count = 0)
//     i.e. rows that already carry a non-zero value are protected by the WHERE
//     clause itself, not by a check in Go that a future edit could drop.
//   - --dry-run classifies every CSV id (will-update / protected / unchanged /
//     missing) and writes nothing -- not even the progress ledger.
//   - Resume: ids are appended to <csv>.progress only after their transaction
//     commits; a lost ledger costs time, never data (the WHERE clause plus the
//     "0 rows affected" reading makes a re-run harmless).
//   - RowsAffected: 1 = written, 0 = protected or already equal (both successful
//     no-ops), >1 = id is not unique -> abort the batch.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"brickplans/internal/db"

	"gorm.io/gorm"
)

const updateSQL = `UPDATE blueprints SET piece_count = ? WHERE id = ? AND (piece_count IS NULL OR piece_count = 0)`

type row struct {
	ID     string
	Pieces int64
	Ln     int
}

// current carries the explicit column tag for the same reason as
// import-longdesc: GORM derives "pieces" from a mistyped field name and
// silently scans NULL, which would make every row look like a candidate.
type current struct {
	ID     string
	Pieces *int64 `gorm:"column:piece_count"`
}

type kept struct {
	r   row
	cur int64
}

type plan struct {
	update    []row
	protected []kept
	same      []row
	missing   []string
}

func classify(rows []row, cur map[string]*current) plan {
	var p plan
	for _, r := range rows {
		c, ok := cur[r.ID]
		if !ok {
			p.missing = append(p.missing, r.ID)
			continue
		}
		switch {
		case c.Pieces == nil || *c.Pieces == 0:
			p.update = append(p.update, r)
		case *c.Pieces == r.Pieces:
			p.same = append(p.same, r)
		default:
			p.protected = append(p.protected, kept{r, *c.Pieces})
		}
	}
	return p
}

func main() {
	var (
		csvPath  = flag.String("csv", "", "CSV with columns id,piece_count (required)")
		dsn      = flag.String("dsn", "", "MySQL DSN (defaults to $MYSQL_DSN)")
		dryRun   = flag.Bool("dry-run", false, "classify every row, print the plan, write nothing")
		apply    = flag.Bool("apply", false, "actually write (required unless --dry-run: no silent writes)")
		progress = flag.String("progress", "", "resume ledger (default <csv>.progress); ignored with --dry-run")
		batch    = flag.Int("batch", 200, "rows per transaction / per SELECT")
		limit    = flag.Int("limit", 0, "apply at most N rows this run (0 = all)")
		samples  = flag.Int("samples", 5, "how many sample diffs to print")
	)
	flag.Parse()
	if *csvPath == "" {
		flag.Usage()
		log.Fatal("--csv is required")
	}
	if !*dryRun && !*apply {
		log.Fatal("refusing to guess: pass --dry-run (plan only) or --apply (write)")
	}
	if *dsn == "" {
		*dsn = os.Getenv("MYSQL_DSN")
	}
	if *dsn == "" {
		log.Fatal("no DSN: pass --dsn or export MYSQL_DSN (run this on the server, backend-go/.env)")
	}
	if *progress == "" {
		*progress = *csvPath + ".progress"
	}

	rows, err := readCSV(*csvPath)
	if err != nil {
		log.Fatalf("csv: %v", err)
	}
	done := loadProgress(*progress, *dryRun)
	log.Printf("csv=%s rows=%d resume-skip=%d dry-run=%v", *csvPath, len(rows), len(done), *dryRun)

	gdb, err := db.Open(*dsn, "production")
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	if err := assertColumns(gdb); err != nil {
		log.Fatalf("%v", err)
	}

	queue := make([]row, 0, len(rows))
	for _, r := range rows {
		if done[r.ID] {
			continue
		}
		queue = append(queue, r)
		if *limit > 0 && len(queue) >= *limit {
			break
		}
	}

	started := time.Now()
	var updated, protected, unchanged, missing int
	shown := 0
	for i := 0; i < len(queue); i += *batch {
		end := i + *batch
		if end > len(queue) {
			end = len(queue)
		}
		chunk := queue[i:end]
		ids := make([]string, len(chunk))
		for j, r := range chunk {
			ids[j] = r.ID
		}
		cur, err := loadCurrent(gdb, ids)
		if err != nil {
			log.Fatalf("select: %v", err)
		}
		p := classify(chunk, cur)
		missing += len(p.missing)
		for _, id := range p.missing {
			log.Printf("[MISS] %s not in blueprints", id)
		}
		unchanged += len(p.same)
		if *dryRun {
			updated += len(p.update)
			protected += len(p.protected)
			for _, r := range p.update {
				if shown < *samples {
					shown++
					fmt.Printf("\n--- sample %d: %s (csv line %d)\n    piece_count: (NULL/0)  -> %d\n", shown, r.ID, r.Ln, r.Pieces)
				}
			}
			for _, pr := range p.protected {
				fmt.Printf("  [PROTECTED] %s piece_count=%d kept, csv wanted %d (WHERE clause skips it)\n", pr.r.ID, pr.cur, pr.r.Pieces)
			}
			continue
		}
		tx := gdb.Begin()
		if tx.Error != nil {
			log.Fatalf("begin: %v", tx.Error)
		}
		written := make([]row, 0, len(p.update))
		for _, r := range p.update {
			res := tx.Exec(updateSQL, r.Pieces, r.ID)
			if res.Error != nil {
				tx.Rollback()
				log.Fatalf("[FAIL] %s: %v (transaction rolled back, this batch wrote nothing)", r.ID, res.Error)
			}
			if res.RowsAffected > 1 {
				tx.Rollback()
				log.Fatalf("[FAIL] %s: UPDATE touched %d rows, id is not unique", r.ID, res.RowsAffected)
			}
			if res.RowsAffected == 1 {
				updated++
				written = append(written, r)
			} else {
				// Lost a race with another writer, or the row already carried a
				// value: both are successful no-ops by design.
				protected++
				log.Printf("[SKIP] %s rows_affected=0 (already has a value)", r.ID)
			}
		}
		if err := tx.Commit().Error; err != nil {
			log.Fatalf("commit: %v", err)
		}
		appendProgress(*progress, written)
		log.Printf("... %d/%d applied", updated, len(queue))
	}

	fmt.Printf("\n==== done in %s ====\n", time.Since(started).Round(time.Millisecond))
	fmt.Printf("rows in csv        : %d\n", len(rows))
	fmt.Printf("skipped (resume)   : %d\n", len(done))
	fmt.Printf("candidates         : %d\n", len(queue))
	fmt.Printf("will update / wrote: %d\n", updated)
	fmt.Printf("protected (kept)   : %d\n", protected)
	fmt.Printf("unchanged          : %d\n", unchanged)
	fmt.Printf("missing ids        : %d\n", missing)
	if *dryRun {
		fmt.Println("wrote: nothing (--dry-run; no progress file touched)")
	} else {
		fmt.Printf("ledger: %s\n", *progress)
	}
}

func readCSV(path string) ([]row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cr := csv.NewReader(f)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	recs, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	out := make([]row, 0, len(recs))
	seen := map[string]bool{}
	for i, rec := range recs {
		if len(rec) == 0 {
			continue
		}
		// The pipeline writes UTF-8 with BOM: without this trim the header check
		// fails and every row is rejected as "not a blueprint id".
		id := strings.TrimSpace(strings.TrimPrefix(rec[0], "\ufeff"))
		if i == 0 && strings.EqualFold(id, "id") {
			continue // header
		}
		if len(rec) < 2 {
			return nil, fmt.Errorf("line %d: want 2 columns (id,piece_count), got %d", i+1, len(rec))
		}
		if len(id) != 32 && len(id) != 36 {
			return nil, fmt.Errorf("line %d: %q is not a blueprint id", i+1, id)
		}
		raw := strings.TrimSpace(rec[1])
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("line %d (%s): piece_count %q is not an integer", i+1, id, raw)
		}
		if n <= 0 {
			return nil, fmt.Errorf("line %d (%s): piece_count %d must be positive (0/empty means unknown, do not import it)", i+1, id, n)
		}
		if seen[id] {
			return nil, fmt.Errorf("line %d: duplicate id %s", i+1, id)
		}
		seen[id] = true
		out = append(out, row{ID: id, Pieces: n, Ln: i + 1})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no data rows")
	}
	return out, nil
}

// assertColumns fails before any work if piece_count is missing, instead of
// surfacing a 1054 from the first UPDATE.
func assertColumns(gdb *gorm.DB) error {
	var n int64
	err := gdb.Raw(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'blueprints'
		  AND column_name = 'piece_count'`).Scan(&n).Error
	if err != nil {
		return fmt.Errorf("check columns: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("blueprints.piece_count is missing")
	}
	return nil
}

func loadCurrent(gdb *gorm.DB, ids []string) (map[string]*current, error) {
	var rows []current
	if err := gdb.Raw(`SELECT id, piece_count FROM blueprints WHERE id IN ?`, ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]*current, len(rows))
	for i := range rows {
		out[rows[i].ID] = &rows[i]
	}
	return out, nil
}

func loadProgress(path string, dryRun bool) map[string]bool {
	out := map[string]bool{}
	if dryRun {
		return out // --dry-run must not even read the ledger, let alone write it
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		if f := strings.Fields(line); len(f) > 0 {
			out[f[0]] = true
		}
	}
	return out
}

func appendProgress(path string, chunk []row) {
	if len(chunk) == 0 {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatalf("progress: %v", err)
	}
	defer f.Close()
	lines := make([]string, 0, len(chunk))
	for _, r := range chunk {
		lines = append(lines, fmt.Sprintf("%s\t%s", r.ID, time.Now().UTC().Format(time.RFC3339)))
	}
	if _, err := f.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		log.Fatalf("progress write: %v", err)
	}
	if err := f.Sync(); err != nil {
		log.Fatalf("progress sync: %v", err)
	}
}
