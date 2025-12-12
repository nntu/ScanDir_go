// checkdup.go
//go:build checkdup

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

type dupGroupRow struct {
	HashValue string
	FileCount int64
	TotalSize int64
	FirstSeen time.Time
}

func parseSQLiteTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		// SQLite TEXT với timezone offset (có dấu cách thay vì 'T')
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05Z07:00",
	}
	var lastErr error
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time %q: %w", s, lastErr)
}

func configureDBForCheckDup(db *sql.DB) {
	// Tối ưu nhẹ cho job vừa đọc vừa ghi
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	_, _ = db.Exec("PRAGMA journal_mode = WAL")
	_, _ = db.Exec("PRAGMA synchronous = NORMAL")
	_, _ = db.Exec("PRAGMA temp_store = MEMORY")
	_, _ = db.Exec("PRAGMA cache_size = -128000")  // 128MB
	_, _ = db.Exec("PRAGMA mmap_size = 536870912") // 512MB
	_, _ = db.Exec("PRAGMA busy_timeout = 5000")
	_, _ = db.Exec("PRAGMA foreign_keys = ON")
}

func ensureDuplicateProgressTables(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS duplicate_groups (
		  hash_value TEXT PRIMARY KEY,
		  file_count INTEGER NOT NULL,
		  total_size BIGINT NOT NULL,
		  first_seen DATETIME NOT NULL,
		  last_updated DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_duplicate_groups_size ON duplicate_groups (total_size DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_duplicate_groups_count ON duplicate_groups (file_count DESC)`,

		`CREATE TABLE IF NOT EXISTS duplicate_runs (
		  id INTEGER PRIMARY KEY AUTOINCREMENT,
		  started_at DATETIME NOT NULL,
		  finished_at DATETIME NULL,
		  status TEXT NOT NULL,
		  total_groups INTEGER DEFAULT 0,
		  processed_groups INTEGER DEFAULT 0,
		  processed_files INTEGER DEFAULT 0,
		  processed_size BIGINT DEFAULT 0,
		  last_hash_value TEXT NULL,
		  note TEXT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_duplicate_runs_status ON duplicate_runs (status)`,
		`CREATE INDEX IF NOT EXISTS idx_duplicate_runs_started_at ON duplicate_runs (started_at DESC)`,
	}

	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func countDuplicateGroups(ctx context.Context, db *sql.DB, fromHash string) (int64, error) {
	// Đếm số group duplicate để hiển thị progress
	var total int64
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM (
			SELECT 1
			FROM fs_files
			WHERE hash_value IS NOT NULL AND hash_value != '' AND hash_value > ?
			GROUP BY hash_value
			HAVING COUNT(*) > 1
		) t
	`, fromHash).Scan(&total)
	return total, err
}

func startRun(ctx context.Context, db *sql.DB, totalGroups int64, note string) (int64, error) {
	res, err := db.ExecContext(ctx, `
		INSERT INTO duplicate_runs (started_at, status, total_groups, note)
		VALUES (?, 'running', ?, ?)
	`, time.Now(), totalGroups, note)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func finishRun(ctx context.Context, db *sql.DB, runID int64, status string, lastHash sql.NullString) {
	_, _ = db.ExecContext(ctx, `
		UPDATE duplicate_runs
		SET finished_at = ?, status = ?, last_hash_value = ?
		WHERE id = ?
	`, time.Now(), status, lastHash, runID)
}

func resetDuplicates(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE fs_files SET is_duplicate = 0 WHERE is_duplicate = 1`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM duplicate_groups`); err != nil {
		return err
	}
	return tx.Commit()
}

func buildInPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}

func commitDupBatch(ctx context.Context, db *sql.DB, runID int64, batch []dupGroupRow, processedGroups *int64, processedFiles *int64, processedSize *int64, lastHash *sql.NullString) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	ins, err := tx.PrepareContext(ctx, `
		INSERT INTO duplicate_groups (hash_value, file_count, total_size, first_seen, last_updated)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(hash_value) DO UPDATE SET
		  file_count = excluded.file_count,
		  total_size = excluded.total_size,
		  first_seen = excluded.first_seen,
		  last_updated = excluded.last_updated
	`)
	if err != nil {
		return err
	}
	defer ins.Close()

	now := time.Now()
	hashes := make([]any, 0, len(batch))

	for _, g := range batch {
		if _, err := ins.ExecContext(ctx, g.HashValue, g.FileCount, g.TotalSize, g.FirstSeen, now); err != nil {
			return err
		}
		hashes = append(hashes, g.HashValue)
		*processedGroups++
		*processedFiles += g.FileCount
		*processedSize += g.TotalSize
		*lastHash = sql.NullString{String: g.HashValue, Valid: true}
	}

	// Mark is_duplicate theo batch group hash_value
	if len(hashes) > 0 {
		q := fmt.Sprintf(`UPDATE fs_files SET is_duplicate = 1 WHERE hash_value IN (%s)`, buildInPlaceholders(len(hashes)))
		if _, err := tx.ExecContext(ctx, q, hashes...); err != nil {
			return err
		}
	}

	// Update progress snapshot (ghi mỗi batch để có thể theo dõi realtime)
	_, err = tx.ExecContext(ctx, `
		UPDATE duplicate_runs
		SET processed_groups = ?, processed_files = ?, processed_size = ?, last_hash_value = ?
		WHERE id = ?
	`, *processedGroups, *processedFiles, *processedSize, *lastHash, runID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func runCheckDup(ctx context.Context, db *sql.DB, dbFile string, reset bool, fromHash string, batchSize int, progressEvery int) error {
	if err := ensureDuplicateProgressTables(ctx, db); err != nil {
		return fmt.Errorf("ensure tables: %w", err)
	}

	if reset {
		log.Printf("Reset duplicate state: is_duplicate=0, clear duplicate_groups ...")
		if err := resetDuplicates(ctx, db); err != nil {
			return fmt.Errorf("reset duplicates: %w", err)
		}
	}

	totalGroups, err := countDuplicateGroups(ctx, db, fromHash)
	if err != nil {
		return fmt.Errorf("count groups: %w", err)
	}

	runID, err := startRun(ctx, db, totalGroups, fmt.Sprintf("dbfile=%s reset=%v fromHash=%q", dbFile, reset, fromHash))
	if err != nil {
		return fmt.Errorf("start run: %w", err)
	}

	var lastHash sql.NullString
	status := "failed"
	defer func() { finishRun(ctx, db, runID, status, lastHash) }()

	log.Printf("Start checkdup run_id=%d total_groups=%d ...", runID, totalGroups)

	rows, err := db.QueryContext(ctx, `
		SELECT hash_value, COUNT(*) as file_count, SUM(size) as total_size, MIN(st_mtime) as first_seen
		FROM fs_files
		WHERE hash_value IS NOT NULL AND hash_value != '' AND hash_value > ?
		GROUP BY hash_value
		HAVING COUNT(*) > 1
		ORDER BY hash_value
	`, fromHash)
	if err != nil {
		return fmt.Errorf("query groups: %w", err)
	}
	defer rows.Close()

	var (
		processedGroups int64
		processedFiles  int64
		processedSize   int64
		batch           = make([]dupGroupRow, 0, batchSize)
		startTime       = time.Now()
	)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := commitDupBatch(ctx, db, runID, batch, &processedGroups, &processedFiles, &processedSize, &lastHash); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}

	for rows.Next() {
		var g dupGroupRow
		var firstSeenRaw sql.NullString
		if err := rows.Scan(&g.HashValue, &g.FileCount, &g.TotalSize, &firstSeenRaw); err != nil {
			return fmt.Errorf("scan group row: %w", err)
		}
		if firstSeenRaw.Valid {
			if t, err := parseSQLiteTime(firstSeenRaw.String); err == nil {
				g.FirstSeen = t
			} else {
				// Không fail cả job chỉ vì parse time; fallback now và log warn.
				g.FirstSeen = time.Now()
				log.Printf("WARN: cannot parse first_seen=%q for hash=%s: %v", firstSeenRaw.String, g.HashValue, err)
			}
		} else {
			g.FirstSeen = time.Now()
		}
		batch = append(batch, g)

		if len(batch) >= batchSize {
			if err := flush(); err != nil {
				return fmt.Errorf("commit batch: %w", err)
			}
		}

		if progressEvery > 0 && processedGroups > 0 && processedGroups%int64(progressEvery) == 0 {
			elapsed := time.Since(startTime)
			speed := float64(processedGroups) / elapsed.Seconds()
			var pct float64
			if totalGroups > 0 {
				pct = float64(processedGroups) * 100 / float64(totalGroups)
			}
			log.Printf("Progress: groups=%d/%d (%.1f%%) files=%d size=%.2fGB speed=%.1f groups/s last=%s",
				processedGroups, totalGroups, pct, processedFiles, float64(processedSize)/(1024*1024*1024), speed, lastHash.String)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate groups: %w", err)
	}
	if err := flush(); err != nil {
		return fmt.Errorf("final commit: %w", err)
	}

	// Done
	status = "done"
	log.Printf("DONE: run_id=%d groups=%d files=%d size=%.2fGB last=%s",
		runID, processedGroups, processedFiles, float64(processedSize)/(1024*1024*1024), lastHash.String)

	return nil
}

func main() {
	dbFile := flag.String("dbfile", "", "Path to the scan.db file (e.g., ./output_scans/scan_....db)")
	reset := flag.Bool("reset", true, "Reset previous duplicate markings (is_duplicate=0, clear duplicate_groups) before rebuilding")
	fromHash := flag.String("from-hash", "", "Start from hash_value > this value (useful to resume manually)")
	batchSize := flag.Int("batch", 500, "Batch size (number of duplicate groups per transaction)")
	progressEvery := flag.Int("progress", 2000, "Log progress every N processed groups (0 to disable)")
	flag.Parse()

	if *dbFile == "" {
		flag.Usage()
		os.Exit(2)
	}
	if *batchSize <= 0 {
		log.Fatal("batch must be > 0")
	}

	ctx := context.Background()
	db, err := openDBSQLite(*dbFile)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	configureDBForCheckDup(db)

	if err := runCheckDup(ctx, db, *dbFile, *reset, *fromHash, *batchSize, *progressEvery); err != nil {
		log.Fatalf("checkdup failed: %v", err)
	}
}


