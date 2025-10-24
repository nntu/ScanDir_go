// scanner.go
//go:build scanner

package main

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// =================================================================
// PHASE 1: SCANNING (METADATA)
// =================================================================

// dbWriter (cho scanner Phase 1)
func dbWriter(ctx context.Context, db *sql.DB, cfg *Config, rx <-chan DbMsg, ready chan<- bool) {
	if err := initDDL(ctx, db); err != nil {
		log.Fatalf("init DDL failed: %v", err)
	}
	log.Println("Phase 1: Database schema initialized.")
	ready <- true
	close(ready)

	fileBatch := make([]FileRow, 0, cfg.BatchSize)

	insertFolderStmt, err := db.PrepareContext(ctx, `
		INSERT INTO fs_folders (parent_id, path, name, st_mtime, loaithumuc)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
		  parent_id=excluded.parent_id, st_mtime=excluded.st_mtime
	`)
	if err != nil {
		log.Fatalf("Failed to prepare folder statement: %v", err)
	}
	defer insertFolderStmt.Close()

	flushFiles := func(rows []FileRow) error {
		if len(rows) == 0 {
			return nil
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO fs_files (folder_id, path, dir_path, filename, fileExt, size, st_mtime, loaithumuc, thumuc)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(path) DO UPDATE SET
			  folder_id=excluded.folder_id, size=excluded.size, st_mtime=excluded.st_mtime
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, r := range rows {
			_, err := stmt.ExecContext(ctx,
				r.FolderID, r.Path, r.DirPath, r.Filename, r.FileExt, r.Size,
				r.Mtime, r.LoaiThuMuc, r.ThuMuc,
			)
			if err != nil {
				log.Printf("WARN: Failed to insert file %s: %v", r.Path, err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("batch commit failed: %w", err)
		}
		log.Printf("Phase 1: flushFiles upserted %d files", len(rows))
		return nil
	}

	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()

loop:
	for {
		select {
		case <-ctx.Done():
			_ = flushFiles(fileBatch)
			break loop
		case m, ok := <-rx:
			if !ok {
				_ = flushFiles(fileBatch)
				break loop
			}

			if m.InsertDir != nil {
				req := m.InsertDir
				var parent sql.NullInt64
				if req.ParentID > 0 {
					parent.Int64 = req.ParentID
					parent.Valid = true
				}
				res, err := insertFolderStmt.ExecContext(ctx,
					parent, req.EntryPath, req.EntryName, req.Info.Mtime, req.LoaiThuMuc,
				)
				if err != nil {
					log.Printf("WARN: Failed to insert folder %s: %v", req.EntryPath, err)
					req.Resp <- -1
					continue
				}

				id, _ := res.LastInsertId()
				if id == 0 {
					_ = db.QueryRowContext(ctx, "SELECT id FROM fs_folders WHERE path = ?", req.EntryPath).Scan(&id)
				}
				req.Resp <- id
			}

			if len(m.InsertFiles) > 0 {
				fileBatch = append(fileBatch, m.InsertFiles...)
				if len(fileBatch) >= cfg.BatchSize {
					if err := flushFiles(fileBatch); err != nil {
						log.Printf("flush files: %v", err)
					}
					fileBatch = fileBatch[:0]
				}
			}

			if m.Shutdown {
				_ = flushFiles(fileBatch)
				fileBatch = fileBatch[:0]
				break loop
			}

		case <-tick.C:
			_ = flushFiles(fileBatch)
			fileBatch = fileBatch[:0]
		}
	}
	log.Println("Phase 1: dbWriter shutting down.")
}

// frame (struct cho scanner)
type frame struct {
	path     string
	folderID int64 // ID của thư mục (từ fs_folders)
	ents     []os.DirEntry
	idx      int
}

// scanRoot (cho scanner Phase 1)
func scanRoot(root, tag string, tx chan<- DbMsg, exclude map[string]struct{}, batchSize int) (uint64, error) {
	abs := root
	if p, err := filepath.Abs(root); err == nil {
		abs = p
	}
	fi, err := os.Lstat(abs)
	if err != nil || !fi.IsDir() {
		return 0, nil
	}
	info := statInfo(fi)

	respRoot := make(chan int64, 1)
	tx <- DbMsg{InsertDir: &DirInsertReq{
		ParentID:   0,
		EntryPath:  abs,
		EntryName:  filepath.Base(strings.TrimRight(abs, string(os.PathSeparator))),
		Info:       info,
		LoaiThuMuc: tag,
		Resp:       respRoot,
	}}
	rootID := <-respRoot
	if rootID <= 0 {
		return 0, fmt.Errorf("failed to insert root folder: %s", abs)
	}

	var totalFiles uint64 = 0
	filesBatch := make([]FileRow, 0, batchSize)
	stack := []frame{}

	ents, err := os.ReadDir(abs)
	if err != nil {
		log.Printf("ERROR: cannot read root dir %s: %v", abs, err)
		return 0, err
	}
	stack = append(stack, frame{path: abs, folderID: rootID, ents: ents, idx: 0})

	for len(stack) > 0 {
		top := &stack[len(stack)-1]

		if top.idx >= len(top.ents) {
			stack = stack[:len(stack)-1]
			continue
		}

		de := top.ents[top.idx]
		top.idx++

		name := de.Name()
		if de.IsDir() {
			if _, skip := exclude[name]; skip {
				continue
			}
		}

		p := filepath.Join(top.path, name)
		fi, err := os.Lstat(p)
		if err != nil {
			log.Printf("WARN: Lstat failed for %s: %v", p, err)
			continue
		}
		inf := statInfo(fi)

		if fi.IsDir() {
			respChild := make(chan int64, 1)
			tx <- DbMsg{InsertDir: &DirInsertReq{
				ParentID:   top.folderID,
				EntryPath:  p,
				EntryName:  name,
				Info:       inf,
				LoaiThuMuc: tag,
				Resp:       respChild,
			}}
			childID := <-respChild

			if childID > 0 {
				chEnts, err := os.ReadDir(p)
				if err != nil {
					log.Printf("WARN: cannot read dir %s: %v", p, err)
				} else {
					stack = append(stack, frame{path: p, folderID: childID, ents: chEnts, idx: 0})
				}
			}
		} else if fi.Mode().IsRegular() {
			totalFiles++
			dirpath := filepath.Dir(p)
			ext := filepath.Ext(name)

			filesBatch = append(filesBatch, FileRow{
				FolderID:   top.folderID,
				Path:       p,
				DirPath:    dirpath,
				Filename:   name,
				FileExt:    ext,
				Size:       fi.Size(),
				Mtime:      fi.ModTime(),
				LoaiThuMuc: tag,
				ThuMuc:     topFolder(p, 4),
			})

			if len(filesBatch) >= batchSize {
				tx <- DbMsg{InsertFiles: filesBatch}
				filesBatch = filesBatch[:0]
			}
		}
	}

	if len(filesBatch) > 0 {
		tx <- DbMsg{InsertFiles: filesBatch}
	}
	return totalFiles, nil
}

// =================================================================
// PHASE 2: HASHING (DUPLICATES)
// =================================================================

// calculateHash (dùng cho Phase 2)
func calculateHash(filePath string) (sql.NullString, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return sql.NullString{}, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return sql.NullString{}, err
	}
	if fi.Size() == 0 {
		return sql.NullString{Valid: false}, nil
	}

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return sql.NullString{}, err
	}

	hashStr := hex.EncodeToString(h.Sum(nil))
	return sql.NullString{String: hashStr, Valid: true}, nil
}

// hashWorker (dùng cho Phase 2)
func hashWorker(jobs <-chan FileToHash, results chan<- HashResult) {
	for job := range jobs {
		hash, err := calculateHash(job.Path)
		if err != nil {
			log.Printf("WARN: Failed to hash %s (ID: %d): %v", job.Path, job.ID, err)
		}
		results <- HashResult{ID: job.ID, Hash: hash, Err: err}
	}
}

// runHashingPhase (Phase 2)
func runHashingPhase(ctx context.Context, db *sql.DB, cfg *Config) {
	log.Println("-------------------------------------------------------")
	log.Println("Phase 2: Hashing potential duplicates starting...")

	// Cho phép nhiều kết nối để băm
	db.SetMaxOpenConns(cfg.MaxWorkers + 1)

	// 1. Tìm tất cả các nhóm file "nghi ngờ" (cùng kích thước)
	log.Println("Phase 2: Finding potential duplicates (groups of same-sized files)...")
	rows, err := db.QueryContext(ctx, `
		SELECT size, COUNT(*) as count, GROUP_CONCAT(id)
		FROM fs_files
		WHERE size > 0 AND hash_value IS NULL
		GROUP BY size
		HAVING count > 1
	`)
	if err != nil {
		log.Fatalf("Phase 2: Failed to query suspect groups: %v", err)
	}
	defer rows.Close()

	var suspectJobs []FileToHash
	var totalSuspects int64 = 0

	for rows.Next() {
		var size int64
		var count int
		var ids string
		if err := rows.Scan(&size, &count, &ids); err != nil {
			log.Fatalf("Phase 2: Failed to scan suspect group: %v", err)
		}

		idRows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT id, path FROM fs_files WHERE id IN (%s)", ids))
		if err != nil {
			log.Printf("WARN: Failed to query paths for group (size %d): %v", size, err)
			continue
		}

		for idRows.Next() {
			var job FileToHash
			if err := idRows.Scan(&job.ID, &job.Path); err != nil {
				log.Printf("WARN: Failed to scan path: %v", err)
				continue
			}
			suspectJobs = append(suspectJobs, job)
			totalSuspects++
		}
		idRows.Close()
	}
	rows.Close()

	if totalSuspects == 0 {
		log.Println("Phase 2: No potential duplicates found. Hashing complete.")
		log.Println("-------------------------------------------------------")
		return
	}

	log.Printf("Phase 2: Found %d files in %d groups needing hashing.", totalSuspects, len(suspectJobs))

	// 2. Khởi tạo Worker Pool để Hashing
	jobs := make(chan FileToHash, totalSuspects)
	results := make(chan HashResult, totalSuspects)

	for w := 0; w < cfg.MaxWorkers; w++ {
		go hashWorker(jobs, results)
	}

	// 3. Gửi jobs
	for _, job := range suspectJobs {
		jobs <- job
	}
	close(jobs)

	// 4. Thu thập kết quả và Ghi vào DB
	log.Println("Phase 2: Hashing in progress...")

	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("Phase 2: Failed to begin update transaction: %v", err)
	}
	defer tx.Rollback()

	updateStmt, err := tx.PrepareContext(ctx, `UPDATE fs_files SET hash_value = ? WHERE id = ?`)
	if err != nil {
		log.Fatalf("Phase 2: Failed to prepare update statement: %v", err)
	}
	defer updateStmt.Close()

	var updatedCount int64 = 0
	for i := int64(0); i < totalSuspects; i++ {
		res := <-results
		if res.Err == nil && res.Hash.Valid {
			_, err := updateStmt.ExecContext(ctx, res.Hash.String, res.ID)
			if err != nil {
				log.Printf("WARN: Failed to update hash for ID %d: %v", res.ID, err)
			} else {
				updatedCount++
			}
		}

		if (i+1)%100 == 0 || i+1 == totalSuspects {
			log.Printf("Phase 2: Progress: %d / %d files hashed...", i+1, totalSuspects)
		}
	}

	if err := tx.Commit(); err != nil {
		log.Fatalf("Phase 2: Failed to commit hash updates: %v", err)
	}

	log.Printf("Phase 2: Hashing complete. Updated %d file hashes.", updatedCount)
	log.Println("-------------------------------------------------------")
}

// =================================================================
// MAIN
// =================================================================

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("Go Scanner (2-Phase: Scan + Hash) starting... Go:%s OS:%s Arch:%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)

	cfg, err := loadConfig("config.ini")
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	dbName := fmt.Sprintf("scan_%s.db", time.Now().Format("20060102_150405"))
	dbPath := filepath.Join(cfg.OutputDir, dbName)
	log.Printf("Output database: %s", dbPath)

	db, err := makeDBSQLite(dbPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// --- BẮT ĐẦU PHASE 1 ---
	log.Println("-------------------------------------------------------")
	log.Println("Phase 1: Scanning metadata starting...")
	rx := make(chan DbMsg, 2048)
	ready := make(chan bool, 1)
	go dbWriter(ctx, db, cfg, rx, ready)

	<-ready // Chờ DB sẵn sàng

	sem := make(chan struct{}, cfg.MaxWorkers)
	var wg sync.WaitGroup
	var totalFiles uint64 = 0
	var mu sync.Mutex

	for _, rt := range cfg.Paths {
		root, tag := rt[0], rt[1]
		if cfg.Paths == nil || len(cfg.Paths) == 0 {
			log.Fatal("Lỗi: [paths] không được định nghĩa trong config.ini")
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(root, tag string) {
			defer wg.Done()
			defer func() { <-sem }()
			if count, err := scanRoot(root, tag, rx, cfg.Exclude, cfg.BatchSize); err != nil {
				log.Printf("Phase 1: scan %s error: %v", root, err)
			} else {
				log.Printf("Phase 1: done %s total files found %d", root, count)
				mu.Lock()
				totalFiles += count
				mu.Unlock()
			}
		}(root, tag)
	}

	wg.Wait()
	rx <- DbMsg{Shutdown: true}
	close(rx)
	log.Printf("Phase 1: All metadata scanning done. Total files: %d.", totalFiles)
	log.Println("-------------------------------------------------------")
	// --- KẾT THÚC PHASE 1 ---

	// --- BẮT ĐẦU PHASE 2 ---
	runHashingPhase(ctx, db, cfg)
	// --- KẾT THÚC PHASE 2 ---

	log.Printf("All tasks complete. Database saved to %s", dbPath)
	log.Println("Bạn có thể dùng tool (ví dụ DBeaver) để mở file .db và truy vấn.")
}
