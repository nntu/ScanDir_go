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
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

// =================================================================
// OPTIMIZED COMPONENTS
// =================================================================

// ScannerLogger provides structured logging capabilities
type ScannerLogger struct {
	logger *logrus.Logger
}

// NewScannerLogger creates a new structured logger
func NewScannerLogger() *ScannerLogger {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetLevel(logrus.InfoLevel)
	return &ScannerLogger{logger: logger}
}

// LogFileProcessed logs file processing metrics
func (sl *ScannerLogger) LogFileProcessed(path string, size int64, duration time.Duration) {
	sl.logger.WithFields(logrus.Fields{
		"path":       path,
		"size":       size,
		"duration":   duration.Milliseconds(),
		"throughput": float64(size) / duration.Seconds() / 1024 / 1024, // MB/s
	}).Info("File processed")
}

// LogBatchOperation logs batch operation metrics
func (sl *ScannerLogger) LogBatchOperation(operation string, count int, duration time.Duration, err error) {
	fields := logrus.Fields{
		"operation": operation,
		"count":     count,
		"duration":  duration.Milliseconds(),
	}

	if err != nil {
		fields["error"] = err.Error()
		sl.logger.WithFields(fields).Error("Batch operation failed")
	} else {
		sl.logger.WithFields(fields).Info("Batch operation completed")
	}
}

// RetryableOperation implements exponential backoff retry mechanism
type RetryableOperation struct {
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
}

// NewRetryableOperation creates a new retryable operation with default settings
func NewRetryableOperation() *RetryableOperation {
	return &RetryableOperation{
		maxRetries: 3,
		baseDelay:  100 * time.Millisecond,
		maxDelay:   5 * time.Second,
	}
}

// Execute runs the operation with retry logic
func (ro *RetryableOperation) Execute(fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= ro.maxRetries; attempt++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err

			if attempt < ro.maxRetries {
				delay := time.Duration(float64(ro.baseDelay) * math.Pow(2, float64(attempt)))
				if delay > ro.maxDelay {
					delay = ro.maxDelay
				}

				time.Sleep(delay)
			}
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", ro.maxRetries+1, lastErr)
}

// BatchSizer implements dynamic batch sizing based on file sizes
type BatchSizer struct {
	targetSize   int64 // Target total size per batch (e.g., 100MB)
	minBatch     int   // Minimum number of files per batch
	maxBatch     int   // Maximum number of files per batch
	currentSize  int64
	currentCount int
}

// NewBatchSizer creates a new batch sizer
func NewBatchSizer(targetSizeMB int64, minBatch, maxBatch int) *BatchSizer {
	return &BatchSizer{
		targetSize: targetSizeMB * 1024 * 1024, // Convert MB to bytes
		minBatch:   minBatch,
		maxBatch:   maxBatch,
	}
}

// ShouldFlush determines if a batch should be flushed based on size and count
func (bs *BatchSizer) ShouldFlush(fileSize int64) bool {
	bs.currentSize += fileSize
	bs.currentCount++

	return bs.currentCount >= bs.maxBatch ||
		   bs.currentSize >= bs.targetSize ||
		   (bs.currentCount >= bs.minBatch && bs.currentSize >= bs.targetSize/2)
}

// Reset resets the batch sizer state
func (bs *BatchSizer) Reset() {
	bs.currentSize = 0
	bs.currentCount = 0
}

// MemoryAwareWorkerPool implements a worker pool with memory management
type MemoryAwareWorkerPool struct {
	workers      int
	jobChan      chan FileToHash
	resultChan   chan HashResult
	done         chan struct{}
	memLimit     int64
	logger       *ScannerLogger
}

// NewMemoryAwareWorkerPool creates a new memory-aware worker pool
func NewMemoryAwareWorkerPool(workers int, memLimitMB int64, logger *ScannerLogger) *MemoryAwareWorkerPool {
	return &MemoryAwareWorkerPool{
		workers:    workers,
		jobChan:    make(chan FileToHash, workers*2),
		resultChan: make(chan HashResult, workers*2),
		done:       make(chan struct{}),
		memLimit:   memLimitMB * 1024 * 1024, // Convert MB to bytes
		logger:     logger,
	}
}

// Start initializes the worker pool
func (wp *MemoryAwareWorkerPool) Start() {
	for i := 0; i < wp.workers; i++ {
		go wp.worker()
	}
}

// worker processes jobs with memory awareness
func (wp *MemoryAwareWorkerPool) worker() {
	defer func() {
		if r := recover(); r != nil {
			wp.logger.logger.Errorf("Worker panic recovered: %v", r)
		}
	}()

	for job := range wp.jobChan {
		// Check memory pressure
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		if m.Alloc > uint64(wp.memLimit) {
			// Force garbage collection
			runtime.GC()
			time.Sleep(10 * time.Millisecond)
		}

		// Process job with timeout
		result := wp.processJobWithTimeout(job, 30*time.Second)
		wp.resultChan <- result
	}
}

// processJobWithTimeout processes a job with a timeout
func (wp *MemoryAwareWorkerPool) processJobWithTimeout(job FileToHash, timeout time.Duration) HashResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Use retry mechanism for file operations
	retryOp := NewRetryableOperation()

	var result HashResult
	err := retryOp.Execute(func() error {
		hash, hashErr := calculateHashWithContext(ctx, job.Path)
		result = HashResult{ID: job.ID, Hash: hash, Err: hashErr}
		return hashErr
	})

	if err != nil {
		wp.logger.logger.WithField("path", job.Path).Warnf("Hash processing failed after retries: %v", err)
	}

	return result
}

// SubmitJob submits a job to the worker pool
func (wp *MemoryAwareWorkerPool) SubmitJob(job FileToHash) {
	wp.jobChan <- job
}

// GetResultChan returns the result channel
func (wp *MemoryAwareWorkerPool) GetResultChan() <-chan HashResult {
	return wp.resultChan
}

// Stop gracefully shuts down the worker pool
func (wp *MemoryAwareWorkerPool) Stop() {
	close(wp.jobChan)
}

// RateLimitedHasher implements I/O rate limiting for hashing operations
type RateLimitedHasher struct {
	semaphore   chan struct{}
	ioTimeout   time.Duration
	maxFileSize int64
	logger      *ScannerLogger
}

// NewRateLimitedHasher creates a new rate-limited hasher
func NewRateLimitedHasher(maxConcurrent int, ioTimeout time.Duration, maxFileSizeMB int64, logger *ScannerLogger) *RateLimitedHasher {
	return &RateLimitedHasher{
		semaphore:   make(chan struct{}, maxConcurrent),
		ioTimeout:   ioTimeout,
		maxFileSize: maxFileSizeMB * 1024 * 1024, // Convert MB to bytes
		logger:      logger,
	}
}

// HashWorker processes hashing jobs with rate limiting
func (rlh *RateLimitedHasher) HashWorker(jobs <-chan FileToHash, results chan<- HashResult) {
	for job := range jobs {
		rlh.semaphore <- struct{}{}
		go func(j FileToHash) {
			defer func() { <-rlh.semaphore }()

			ctx, cancel := context.WithTimeout(context.Background(), rlh.ioTimeout)
			defer cancel()

			hash, err := calculateHashWithContext(ctx, j.Path)
			if err != nil {
				rlh.logger.logger.WithField("path", j.Path).Warnf("Hash calculation failed: %v", err)
			}

			results <- HashResult{ID: j.ID, Hash: hash, Err: err}
		}(job)
	}
}

// getFilesByIDChunked safely retrieves files by ID chunks
func getFilesByIDChunked(ctx context.Context, db *sql.DB, ids []int64) ([]FileToHash, error) {
	const chunkSize = 1000
	var allFiles []FileToHash

	for i := 0; i < len(ids); i += chunkSize {
		end := i + chunkSize
		if end > len(ids) {
			end = len(ids)
		}

		placeholders := strings.Repeat("?,", len(ids[i:end]))
		placeholders = placeholders[:len(placeholders)-1] // Remove trailing comma

		query := fmt.Sprintf("SELECT id, path FROM fs_files WHERE id IN (%s)", placeholders)
		args := make([]interface{}, len(ids[i:end]))
		for j, id := range ids[i:end] {
			args[j] = id
		}

		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to query files chunk: %w", err)
		}

		for rows.Next() {
			var job FileToHash
			if err := rows.Scan(&job.ID, &job.Path); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan file row: %w", err)
			}
			allFiles = append(allFiles, job)
		}
		rows.Close()
	}

	return allFiles, nil
}

// calculateHashWithContext calculates hash with context support
func calculateHashWithContext(ctx context.Context, filePath string) (sql.NullString, error) {
	// Check if file exists and get size
	f, err := os.Open(filePath)
	if err != nil {
		return sql.NullString{}, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return sql.NullString{}, err
	}

	// Skip empty files
	if fi.Size() == 0 {
		return sql.NullString{Valid: false}, nil
	}

	h := md5.New()

	// Use buffered reading for better performance
	buf := make([]byte, 64*1024) // 64KB buffer

	for {
		select {
		case <-ctx.Done():
			return sql.NullString{}, ctx.Err()
		default:
			n, err := f.Read(buf)
			if n > 0 {
				if _, err := h.Write(buf[:n]); err != nil {
					return sql.NullString{}, err
				}
			}
			if err == io.EOF {
				goto done
			}
			if err != nil {
				return sql.NullString{}, err
			}
		}
	}

done:
	hashStr := hex.EncodeToString(h.Sum(nil))
	return sql.NullString{String: hashStr, Valid: true}, nil
}

// =================================================================
// PHASE 1: SCANNING (METADATA)
// =================================================================

// dbWriterOptimized (cho scanner Phase 1 - Optimized Version)
func dbWriterOptimized(ctx context.Context, db *sql.DB, cfg *Config, rx <-chan DbMsg, ready chan<- bool) {
	logger := NewScannerLogger()

	// Configure database for scan phase
	configureDB(db, "scan", cfg.MaxWorkers)

	if err := initDDL(ctx, db); err != nil {
		logger.logger.Fatalf("init DDL failed: %v", err)
	}
	logger.logger.Info("Phase 1: Database schema initialized with optimized indexes.")
	ready <- true
	close(ready)

	// Initialize dynamic batch sizer
	batchSizer := NewBatchSizer(100, 1000, 10000) // 100MB target, 1K-10K files per batch
	fileBatch := make([]FileRow, 0, batchSizer.maxBatch)

	// Initialize retry mechanism for database operations
	retryOp := NewRetryableOperation()

	insertFolderStmt, err := db.PrepareContext(ctx, `
		INSERT INTO fs_folders (parent_id, path, name, st_mtime, loaithumuc)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
		  parent_id=excluded.parent_id, st_mtime=excluded.st_mtime
	`)
	if err != nil {
		logger.logger.Fatalf("Failed to prepare folder statement: %v", err)
	}
	defer insertFolderStmt.Close()

	flushFiles := func(rows []FileRow) error {
		if len(rows) == 0 {
			return nil
		}
		startTime := time.Now()

		return retryOp.Execute(func() error {
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
					logger.logger.WithFields(logrus.Fields{
						"path":  r.Path,
						"error": err.Error(),
					}).Warn("Failed to insert file")
				}
			}
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("batch commit failed: %w", err)
			}

			duration := time.Since(startTime)
			logger.LogBatchOperation("file_insert", len(rows), duration, nil)
			return nil
		})
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

				// Use retry for folder insertion
				var res sql.Result
				var err error

				retryErr := retryOp.Execute(func() error {
					res, err = insertFolderStmt.ExecContext(ctx,
						parent, req.EntryPath, req.EntryName, req.Info.Mtime, req.LoaiThuMuc,
					)
					return err
				})

				if retryErr != nil {
					logger.logger.WithFields(logrus.Fields{
						"path":  req.EntryPath,
						"error": retryErr.Error(),
					}).Warn("Failed to insert folder")
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
				for _, file := range m.InsertFiles {
					fileBatch = append(fileBatch, file)

					// Check if batch should be flushed based on file size
					if batchSizer.ShouldFlush(file.Size) {
						if err := flushFiles(fileBatch); err != nil {
							logger.logger.WithError(err).Error("Failed to flush file batch")
						}
						fileBatch = fileBatch[:0]
						batchSizer.Reset()
					}
				}
			}

			if m.Shutdown {
				_ = flushFiles(fileBatch)
				fileBatch = fileBatch[:0]
				break loop
			}

		case <-tick.C:
			if len(fileBatch) > 0 {
				if err := flushFiles(fileBatch); err != nil {
					logger.logger.WithError(err).Error("Failed to flush files on timer")
				}
				fileBatch = fileBatch[:0]
				batchSizer.Reset()
			}
		}
	}
	logger.logger.Info("Phase 1: dbWriter shutting down.")
}

// dbWriter (legacy function - kept for compatibility)
func dbWriter(ctx context.Context, db *sql.DB, cfg *Config, rx <-chan DbMsg, ready chan<- bool) {
	dbWriterOptimized(ctx, db, cfg, rx, ready)
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

// runHashingPhaseOptimized (Phase 2 - Optimized Version)
func runHashingPhaseOptimized(ctx context.Context, db *sql.DB, cfg *Config) {
	logger := NewScannerLogger()
	logger.logger.Info("-------------------------------------------------------")
	logger.logger.Info("Phase 2: Hashing potential duplicates starting...")

	// Configure optimized database connections
	configureDB(db, "hash", cfg.MaxWorkers)

	// 1. Find potential duplicate groups safely
	logger.logger.Info("Phase 2: Finding potential duplicates (groups of same-sized files)...")
	rows, err := db.QueryContext(ctx, `
		SELECT size, COUNT(*) as count
		FROM fs_files
		WHERE size > 0 AND hash_value IS NULL
		GROUP BY size
		HAVING count > 1
	`)
	if err != nil {
		logger.logger.Fatalf("Phase 2: Failed to query suspect groups: %v", err)
	}
	defer rows.Close()

	var suspectJobs []FileToHash
	var totalSuspects int64 = 0

	// 2. Get file IDs using safe parameterized queries
	for rows.Next() {
		var size int64
		var count int
		if err := rows.Scan(&size, &count); err != nil {
			logger.logger.Fatalf("Phase 2: Failed to scan suspect group: %v", err)
		}

		// Get file IDs for this size group using parameterized query
		sizeRows, err := db.QueryContext(ctx,
			"SELECT id, path FROM fs_files WHERE size > 0 AND hash_value IS NULL AND size = ?", size)
		if err != nil {
			logger.logger.WithError(err).WithField("size", size).Warn("Failed to query paths for size group")
			continue
		}

		groupJobs := []FileToHash{}
		for sizeRows.Next() {
			var job FileToHash
			if err := sizeRows.Scan(&job.ID, &job.Path); err != nil {
				logger.logger.WithError(err).Warn("Failed to scan file row")
				continue
			}
			groupJobs = append(groupJobs, job)
			totalSuspects++
		}
		sizeRows.Close()

		suspectJobs = append(suspectJobs, groupJobs...)
	}

	if totalSuspects == 0 {
		logger.logger.Info("Phase 2: No potential duplicates found. Hashing complete.")
		logger.logger.Info("-------------------------------------------------------")
		return
	}

	logger.logger.WithFields(logrus.Fields{
		"totalFiles": totalSuspects,
		"groupCount": len(suspectJobs),
	}).Info("Phase 2: Found files needing hashing")

	// 3. Initialize optimized worker pool with memory awareness
	workerPool := NewMemoryAwareWorkerPool(cfg.MaxWorkers, 1024, logger) // 1GB memory limit
	workerPool.Start()
	defer workerPool.Stop()

	// 4. Initialize rate-limited hasher
	rateLimiter := NewRateLimitedHasher(cfg.MaxWorkers, 30*time.Second, 1024, logger) // 1GB max file size

	// 5. Submit jobs to worker pool
	logger.logger.Info("Phase 2: Submitting jobs to worker pool...")
	jobs := make(chan FileToHash, totalSuspects)
	results := make(chan HashResult, totalSuspects)

	// Start rate-limited hash workers
	for w := 0; w < cfg.MaxWorkers; w++ {
		go rateLimiter.HashWorker(jobs, results)
	}

	// Submit all jobs
	for _, job := range suspectJobs {
		jobs <- job
	}
	close(jobs)

	// 6. Collect results and update database with batch processing
	logger.logger.Info("Phase 2: Collecting hash results and updating database...")

	const batchSize = 100
	var batch []HashResult
	var updatedCount int64 = 0

	tx, err := db.Begin()
	if err != nil {
		logger.logger.Fatalf("Phase 2: Failed to begin update transaction: %v", err)
	}

	updateStmt, err := tx.PrepareContext(ctx, `UPDATE fs_files SET hash_value = ? WHERE id = ?`)
	if err != nil {
		tx.Rollback()
		logger.logger.Fatalf("Phase 2: Failed to prepare update statement: %v", err)
	}
	defer updateStmt.Close()

	for i := int64(0); i < totalSuspects; i++ {
		res := <-results
		if res.Err == nil && res.Hash.Valid {
			batch = append(batch, res)

			// Process batch
			if len(batch) >= batchSize {
				batchUpdated := processHashBatch(ctx, tx, updateStmt, batch, logger)
				updatedCount += int64(batchUpdated)
				batch = batch[:0]
			}
		} else if res.Err != nil {
			logger.logger.WithFields(logrus.Fields{
				"id":    res.ID,
				"error": res.Err.Error(),
			}).Warn("Hash calculation failed")
		}

		// Progress logging
		if (i+1)%100 == 0 || i+1 == totalSuspects {
			logger.logger.WithFields(logrus.Fields{
				"processed": i + 1,
				"total":     totalSuspects,
				"updated":   updatedCount,
			}).Info("Phase 2: Hashing progress")
		}
	}

	// Process remaining batch
	if len(batch) > 0 {
		batchUpdated := processHashBatch(ctx, tx, updateStmt, batch, logger)
		updatedCount += int64(batchUpdated)
	}

	if err := tx.Commit(); err != nil {
		logger.logger.Fatalf("Phase 2: Failed to commit hash updates: %v", err)
	}

	logger.logger.WithFields(logrus.Fields{
		"totalProcessed": totalSuspects,
		"totalUpdated":   updatedCount,
	}).Info("Phase 2: Hashing complete")
	logger.logger.Info("-------------------------------------------------------")
}

// processHashBatch processes a batch of hash updates
func processHashBatch(ctx context.Context, tx *sql.Tx, stmt *sql.Stmt, batch []HashResult, logger *ScannerLogger) int {
	startTime := time.Now()
	updated := 0

	for _, res := range batch {
		_, err := stmt.ExecContext(ctx, res.Hash.String, res.ID)
		if err != nil {
			logger.logger.WithFields(logrus.Fields{
				"id":    res.ID,
				"error": err.Error(),
			}).Warn("Failed to update hash for ID")
		} else {
			updated++
		}
	}

	duration := time.Since(startTime)
	logger.LogBatchOperation("hash_update", updated, duration, nil)

	return updated
}

// configureDB configures database connection settings for optimal performance
func configureDB(db *sql.DB, phase string, workers int) {
	switch phase {
	case "scan":
		// Phase 1: Write-heavy, single connection is optimal
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	case "hash":
		// Phase 2: Read-heavy operations
		db.SetMaxOpenConns(workers + 2)
		db.SetMaxIdleConns(workers)
		db.SetConnMaxLifetime(time.Hour)
		db.SetConnMaxIdleTime(time.Minute * 30)
	case "delete":
		// Deletion: Mixed read/write operations
		db.SetMaxOpenConns(2)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(30 * time.Minute)
	case "report":
		// Reporting: Read-only operations, optimized for complex queries
		db.SetMaxOpenConns(2)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(10 * time.Minute)
	}

	// Optimize SQLite settings for performance
	db.Exec("PRAGMA journal_mode = WAL")
	db.Exec("PRAGMA synchronous = NORMAL")
	db.Exec("PRAGMA cache_size = -64000") // 64MB cache
	db.Exec("PRAGMA temp_store = MEMORY")
	db.Exec("PRAGMA mmap_size = 268435456") // 256MB memory map
	db.Exec("PRAGMA busy_timeout = 5000")

	// Additional optimizations for specific phases
	switch phase {
	case "delete":
		db.Exec("PRAGMA foreign_keys = ON")
	case "report":
		db.Exec("PRAGMA query_only = 1") // Read-only for reporting
		db.Exec("PRAGMA cache_size = -128000") // 128MB cache for reporting
		db.Exec("PRAGMA mmap_size = 536870912") // 512MB memory map for reporting
	}
}

// runHashingPhase (legacy function - kept for compatibility)
func runHashingPhase(ctx context.Context, db *sql.DB, cfg *Config) {
	runHashingPhaseOptimized(ctx, db, cfg)
}

// =================================================================
// DYNAMIC CONFIGURATION
// =================================================================

// DynamicConfig implements runtime configuration adjustment
type DynamicConfig struct {
	*Config
	logger         *ScannerLogger
	memLimit       int64
	lastAdjustment time.Time

	// Runtime tunable parameters
	AdjustedBatchSize int
	AdjustedWorkers  int
	AdjustedTimeout  time.Duration
}

// NewDynamicConfig creates a new dynamic configuration
func NewDynamicConfig(baseCfg *Config, memLimitMB int64, logger *ScannerLogger) *DynamicConfig {
	return &DynamicConfig{
		Config:           baseCfg,
		logger:           logger,
		memLimit:         memLimitMB * 1024 * 1024, // Convert to bytes
		lastAdjustment:   time.Now(),
		AdjustedBatchSize: baseCfg.BatchSize,
		AdjustedWorkers:  baseCfg.MaxWorkers,
		AdjustedTimeout:  30 * time.Second,
	}
}

// AutoAdjust dynamically adjusts configuration based on system conditions
func (dc *DynamicConfig) AutoAdjust() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Only adjust every 30 seconds
	if time.Since(dc.lastAdjustment) < 30*time.Second {
		return
	}

	adjusted := false

	// Adjust batch size based on memory pressure
	if m.Alloc > uint64(float64(dc.memLimit)*0.8) {
		// Reduce batch size if memory pressure is high
		newBatchSize := max(100, dc.AdjustedBatchSize/2)
		if newBatchSize != dc.AdjustedBatchSize {
			dc.logger.logger.WithFields(logrus.Fields{
				"oldBatchSize": dc.AdjustedBatchSize,
				"newBatchSize": newBatchSize,
				"memoryUsage":  m.Alloc,
				"memoryLimit":  dc.memLimit,
			}).Info("Reducing batch size due to memory pressure")
			dc.AdjustedBatchSize = newBatchSize
			adjusted = true
		}
	} else if m.Alloc < uint64(float64(dc.memLimit)*0.4) {
		// Increase batch size if memory usage is low
		newBatchSize := min(10000, dc.AdjustedBatchSize*3/2)
		if newBatchSize != dc.AdjustedBatchSize {
			dc.logger.logger.WithFields(logrus.Fields{
				"oldBatchSize": dc.AdjustedBatchSize,
				"newBatchSize": newBatchSize,
				"memoryUsage":  m.Alloc,
				"memoryLimit":  dc.memLimit,
			}).Info("Increasing batch size due to low memory usage")
			dc.AdjustedBatchSize = newBatchSize
			adjusted = true
		}
	}

	// Adjust worker count based on CPU usage
	if adjusted || time.Since(dc.lastAdjustment) > time.Minute {
		cpuCount := runtime.NumCPU()
		loadPercent := getCPULoad()

		if loadPercent > 80 {
			// Reduce workers if CPU is busy
			newWorkers := max(1, dc.AdjustedWorkers-1)
			if newWorkers != dc.AdjustedWorkers {
				dc.logger.logger.WithFields(logrus.Fields{
					"oldWorkers":  dc.AdjustedWorkers,
					"newWorkers":  newWorkers,
					"cpuLoad":     loadPercent,
				}).Info("Reducing worker count due to high CPU usage")
				dc.AdjustedWorkers = newWorkers
			}
		} else if loadPercent < 40 && dc.AdjustedWorkers < cpuCount*2 {
			// Increase workers if CPU is available
			newWorkers := min(cpuCount*2, dc.AdjustedWorkers+1)
			if newWorkers != dc.AdjustedWorkers {
				dc.logger.logger.WithFields(logrus.Fields{
					"oldWorkers":  dc.AdjustedWorkers,
					"newWorkers":  newWorkers,
					"cpuLoad":     loadPercent,
				}).Info("Increasing worker count due to low CPU usage")
				dc.AdjustedWorkers = newWorkers
			}
		}
	}

	dc.lastAdjustment = time.Now()
}

// getCPULoad estimates CPU load (simplified version)
func getCPULoad() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Use GC pause as a rough indicator of system load
	// This is a simplified approach - in production you might want to use
	// system-specific CPU metrics
	if m.NumGC > 0 {
		return float64(m.PauseTotalNs) / float64(m.NumGC) / float64(time.Millisecond) * 100
	}
	return 0.0
}

// =================================================================
// MAIN - OPTIMIZED VERSION
// =================================================================

func main() {
	// Initialize structured logging
	logger := NewScannerLogger()
	logger.logger.WithFields(logrus.Fields{
		"goVersion": runtime.Version(),
		"os":        runtime.GOOS,
		"arch":      runtime.GOARCH,
		"startTime": time.Now(),
	}).Info("Go Scanner (Optimized 2-Phase: Scan + Hash) starting...")

	// Load configuration
	cfg, err := loadConfig("config.ini")
	if err != nil {
		logger.logger.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize dynamic configuration
	dynamicCfg := NewDynamicConfig(cfg, 2048, logger) // 2GB memory limit

	// Create output database
	dbName := fmt.Sprintf("scan_%s.db", time.Now().Format("20060102_150405"))
	dbPath := filepath.Join(cfg.OutputDir, dbName)
	logger.logger.WithField("dbPath", dbPath).Info("Output database path")

	db, err := makeDBSQLite(dbPath)
	if err != nil {
		logger.logger.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- PHASE 1: METADATA SCANNING ---
	logger.logger.Info("-------------------------------------------------------")
	logger.logger.Info("Phase 1: Scanning metadata starting...")

	// Create optimized message channel with backpressure management
	rx := make(chan DbMsg, 1024)
	ready := make(chan bool, 1)

	// Start optimized database writer
	go dbWriterOptimized(ctx, db, dynamicCfg.Config, rx, ready)

	<-ready // Wait for database to be ready

	// Use optimized semaphore and wait group
	sem := make(chan struct{}, dynamicCfg.AdjustedWorkers)
	var wg sync.WaitGroup
	var totalFiles uint64 = 0
	var mu sync.Mutex

	// Validate paths before starting
	if len(cfg.Paths) == 0 {
		logger.logger.Fatal("No paths configured in config.ini")
	}

	// Start periodic configuration adjustment
	adjustTicker := time.NewTicker(1 * time.Minute)
	defer adjustTicker.Stop()

	// Launch scanner for each path
	for _, rt := range cfg.Paths {
		root, tag := rt[0], rt[1]

		wg.Add(1)
		sem <- struct{}{}
		go func(root, tag string) {
			defer wg.Done()
			defer func() { <-sem }()

			// Log the start of scanning for this path
			logger.logger.WithFields(logrus.Fields{
				"path": root,
				"tag":  tag,
			}).Info("Starting path scan")

			startTime := time.Now()
			if count, err := scanRoot(root, tag, rx, cfg.Exclude, dynamicCfg.AdjustedBatchSize); err != nil {
				logger.logger.WithFields(logrus.Fields{
					"path":  root,
					"error": err.Error(),
				}).Error("Phase 1: scan error")
			} else {
				duration := time.Since(startTime)
				logger.logger.WithFields(logrus.Fields{
					"path":      root,
					"tag":       tag,
					"fileCount": count,
					"duration":  duration.Milliseconds(),
					"throughput": float64(count) / duration.Seconds(),
				}).Info("Phase 1: path scan completed")

				mu.Lock()
				totalFiles += count
				mu.Unlock()
			}
		}(root, tag)
	}

	// Monitor and adjust configuration during scanning
	go func() {
		for {
			select {
			case <-adjustTicker.C:
				dynamicCfg.AutoAdjust()
				logger.logger.WithFields(logrus.Fields{
					"batchSize": dynamicCfg.AdjustedBatchSize,
					"workers":   dynamicCfg.AdjustedWorkers,
				}).Debug("Configuration auto-adjusted")
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for all scanning to complete
	wg.Wait()

	// Signal shutdown to database writer
	rx <- DbMsg{Shutdown: true}
	close(rx)

	logger.logger.WithField("totalFiles", totalFiles).Info("Phase 1: All metadata scanning completed")
	logger.logger.Info("-------------------------------------------------------")
	// --- END PHASE 1 ---

	// --- PHASE 2: HASHING DUPLICATES ---
	logger.logger.Info("Starting Phase 2: Hashing potential duplicates")
	runHashingPhaseOptimized(ctx, db, dynamicCfg.Config)
	// --- END PHASE 2 ---

	// Final performance summary
	logger.logger.WithFields(logrus.Fields{
		"dbPath":     dbPath,
		"totalFiles": totalFiles,
		"endTime":    time.Now(),
	}).Info("All scanning and hashing tasks completed successfully")

	logger.logger.Info("You can use tools like DBeaver to open the database file and run queries.")
}

// Legacy main function for backward compatibility
func mainLegacy() {
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
