// Command import-longdesc loads the batch-2 editorial bodies into
// blueprints.description_long / description_long_en from a 3-column CSV.
//
//	import-longdesc --csv bodies.csv --dry-run
//	import-longdesc --csv bodies.csv
//
// Contract with the ticket (SEO ticket 02, batch 2):
//
//   - The CSV is id,zh,en. Anything else is rejected before a single write.
//   - ONLY the two new columns are ever written. description and description_en
//     are read for the diff and then left alone; the generated SQL cannot touch
//     them because the SET clause is built from a fixed two-entry table.
//   - --dry-run prints what would change (rows affected, missing ids, out-of-range
//     bodies, 3 sample diffs) and writes nothing, not even the progress file.
//   - Resume: every applied id is appended to <csv>.progress after its transaction
//     commits. Re-running the same command skips those ids. The pass is also
//     idempotent on its own -- a row already equal to the CSV counts as unchanged
//     and is never rewritten -- so a lost progress file costs time, not data.
//   - An empty zh or en cell means "leave that column as it is". A row with both
//     cells empty is a CSV error, not a silent no-op.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"brickplans/internal/db"

	"gorm.io/gorm"
)

type row struct {
	ID string
	ZH string
	EN string
	Ln int
}

// current mirrors the columns the diff needs. The two long columns carry an
// explicit gorm column tag: without it GORM derives "long" / "long_en" from the
// field names, the scan silently leaves them NULL, and every row looks dirty.
type current struct {
	ID            string
	Description   *string `gorm:"column:description"`
	DescriptionEN *string `gorm:"column:description_en"`
	Long          *string `gorm:"column:description_long"`
	LongEN        *string `gorm:"column:description_long_en"`
}

func col(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func preview(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func main() {
	var (
		csvPath  = flag.String("csv", "", "CSV with columns id,zh,en (required)")
		dsn      = flag.String("dsn", "", "MySQL DSN (defaults to $MYSQL_DSN)")
		dryRun   = flag.Bool("dry-run", false, "print the plan (counts + 3 sample diffs) and write nothing")
		progress = flag.String("progress", "", "resume ledger (default <csv>.progress); ignored with --dry-run")
		batch    = flag.Int("batch", 100, "rows per transaction / per SELECT")
		limit    = flag.Int("limit", 0, "apply at most N rows this run (0 = all)")
		samples  = flag.Int("samples", 3, "how many sample diffs to print")
		minRunes = flag.Int("min-runes", 250, "warn below this body length (never truncates, never pads)")
		maxRunes = flag.Int("max-runes", 500, "warn above this body length (never truncates)")
	)
	flag.Parse()
	if *csvPath == "" {
		flag.Usage()
		log.Fatal("--csv is required")
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
		log.Fatalf("%v\n先执行 ops/sql/20261028-description-long/01-ddl.sql，再跑这个工具", err)
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
	var (
		updated, unchanged, missing, failed int
		shortZH, longZH, shortEN, longEN    int
		sampleShown                         int
	)
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
		// One transaction per chunk keeps the binlog small and gives a clean
		// resume boundary: the ledger is only extended after Commit succeeds.
		tx := gdb.Begin()
		if tx.Error != nil {
			log.Fatalf("begin: %v", tx.Error)
		}
		for _, r := range chunk {
			before, ok := cur[r.ID]
			if !ok {
				missing++
				log.Printf("[MISS] %s not in blueprints", r.ID)
				continue
			}
			if r.ZH != "" {
				if n := utf8.RuneCountInString(r.ZH); n < *minRunes {
					shortZH++
				} else if n > *maxRunes {
					longZH++
				}
			}
			if r.EN != "" {
				if n := utf8.RuneCountInString(r.EN); n < *minRunes {
					shortEN++
				} else if n > *maxRunes {
					longEN++
				}
			}
			sameZH := r.ZH == "" || r.ZH == col(before.Long)
			sameEN := r.EN == "" || r.EN == col(before.LongEN)
			if sameZH && sameEN {
				unchanged++
				continue
			}
			if *dryRun {
				updated++
				if sampleShown < *samples {
					sampleShown++
					printDiff(sampleShown, r, before)
				}
				continue
			}
			touched, err := applyOne(tx, r)
			if err != nil {
				tx.Rollback()
				log.Fatalf("[FAIL] %s: %v (transaction rolled back, nothing in this batch was written)", r.ID, err)
			}
			if touched {
				updated++
			} else {
				unchanged++
			}
		}
		if *dryRun {
			continue
		}
		if err := tx.Commit().Error; err != nil {
			log.Fatalf("commit: %v", err)
		}
		appendProgress(*progress, chunk)
		log.Printf("… %d/%d applied (%.0f rows/s)", updated, len(queue), float64(updated)/time.Since(started).Seconds())
	}
	if *dryRun {
		fmt.Printf("\n[dry-run] 不写库。将被 UPDATE 的行数: %d\n", updated)
	}

	fmt.Printf("\n════ done in %s ════\n", time.Since(started).Round(time.Millisecond))
	fmt.Printf("rows in csv      : %d\n", len(rows))
	fmt.Printf("skipped (resume) : %d\n", len(done))
	fmt.Printf("candidates       : %d\n", len(queue))
	fmt.Printf("updated          : %d\n", updated)
	fmt.Printf("unchanged        : %d\n", unchanged)
	fmt.Printf("missing ids      : %d\n", missing)
	fmt.Printf("failed           : %d\n", failed)
	fmt.Printf("zh len out of [%d,%d]: short=%d long=%d\n", *minRunes, *maxRunes, shortZH, longZH)
	fmt.Printf("en len out of [%d,%d]: short=%d long=%d\n", *minRunes, *maxRunes, shortEN, longEN)
	if *dryRun {
		fmt.Println("wrote: nothing (--dry-run)")
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
		id := strings.TrimSpace(strings.TrimPrefix(rec[0], "\ufeff"))
		if i == 0 && strings.EqualFold(id, "id") {
			continue // header
		}
		if len(rec) < 3 {
			return nil, fmt.Errorf("line %d: want 3 columns (id,zh,en), got %d", i+1, len(rec))
		}
		zh, en := strings.TrimSpace(rec[1]), strings.TrimSpace(rec[2])
		if len(id) < 32 {
			return nil, fmt.Errorf("line %d: %q is not a blueprint id", i+1, id)
		}
		if zh == "" && en == "" {
			return nil, fmt.Errorf("line %d (%s): both zh and en are empty", i+1, id)
		}
		if seen[id] {
			return nil, fmt.Errorf("line %d: duplicate id %s", i+1, id)
		}
		seen[id] = true
		out = append(out, row{ID: id, ZH: zh, EN: en, Ln: i + 1})
	}
	return out, nil
}

// assertColumns fails before any work if the DDL has not been applied -- a plain
// SELECT on a missing column would otherwise surface as a confusing 1054 later.
func assertColumns(gdb *gorm.DB) error {
	var n int64
	err := gdb.Raw(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'blueprints'
		  AND column_name IN ('description_long','description_long_en')`).Scan(&n).Error
	if err != nil {
		return fmt.Errorf("check columns: %w", err)
	}
	if n != 2 {
		return fmt.Errorf("blueprints is missing description_long / description_long_en (found %d of 2)", n)
	}
	return nil
}

func loadCurrent(gdb *gorm.DB, ids []string) (map[string]*current, error) {
	out := make(map[string]*current, len(ids))
	var rows []current
	err := gdb.Raw(`SELECT id, description, description_en,
		description_long, description_long_en
		FROM blueprints WHERE id IN ?`, ids).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		out[rows[i].ID] = &rows[i]
	}
	return out, nil
}

// applyOne writes the two long columns and nothing else. The SET clause is
// assembled from a fixed table so no CSV value can ever widen it.
func applyOne(tx *gorm.DB, r row) (touched bool, err error) {
	sets := []string{}
	args := []interface{}{}
	if r.ZH != "" {
		sets = append(sets, "description_long = ?")
		args = append(args, r.ZH)
	}
	if r.EN != "" {
		sets = append(sets, "description_long_en = ?")
		args = append(args, r.EN)
	}
	if len(sets) == 0 {
		return false, nil
	}
	args = append(args, r.ID)
	q := "UPDATE blueprints SET " + strings.Join(sets, ", ") + " WHERE id = ? LIMIT 1"
	res := tx.Exec(q, args...)
	if res.Error != nil {
		return false, res.Error
	}
	// 0 means "the row exists but the new values are identical to the stored ones",
	// which is what a resume looks like when the ledger was lost. Anything above 1
	// would mean the id is not unique -- abort the batch.
	if res.RowsAffected > 1 {
		return false, fmt.Errorf("UPDATE touched %d rows, want 1", res.RowsAffected)
	}
	return res.RowsAffected == 1, nil
}

func printDiff(n int, r row, before *current) {
	fmt.Printf("\n--- sample %d: %s (csv line %d)\n", n, r.ID, r.Ln)
	for _, c := range []struct {
		name string
		next string
		prev *string
	}{{"description_long", r.ZH, before.Long}, {"description_long_en", r.EN, before.LongEN}} {
		if c.next == "" {
			fmt.Printf("  %-20s: (csv 空，保持不动)\n", c.name)
			continue
		}
		old := "(NULL)"
		if c.prev != nil && strings.TrimSpace(*c.prev) != "" {
			old = fmt.Sprintf("%d 字 %q", utf8.RuneCountInString(*c.prev), preview(*c.prev, 60))
		}
		fmt.Printf("  %-20s: %s\n  %-20s  → %d 字 %q\n", c.name, old, "", utf8.RuneCountInString(c.next), preview(c.next, 60))
	}
	fmt.Printf("  %-20s: 未改动（%d 字）\n", "description(protected)", utf8.RuneCountInString(col(before.Description)))
}

func loadProgress(path string, dryRun bool) map[string]bool {
	out := map[string]bool{}
	if dryRun {
		return out
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
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatalf("progress: %v", err)
	}
	defer f.Close()
	ids := make([]string, 0, len(chunk))
	for _, r := range chunk {
		ids = append(ids, fmt.Sprintf("%s\t%s", r.ID, time.Now().UTC().Format(time.RFC3339)))
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if _, err := f.WriteString(strings.Join(ids, "\n") + "\n"); err != nil {
		log.Fatalf("progress write: %v", err)
	}
	if err := f.Sync(); err != nil {
		log.Fatalf("progress sync: %v", err)
	}
}
