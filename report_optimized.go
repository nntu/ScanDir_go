// report_optimized.go
//go:build reporter_optimized

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	"github.com/xuri/excelize/v2"
)

// NOTE: openDBSQLite được dùng chung từ `common_db.go` (build tag reporter_optimized đã được bật).

// configureDB configures database connection settings for optimal performance
func configureDB(db *sql.DB, phase string, workers int) {
	switch phase {
	case "report":
		// Reporting: Read-only operations, optimized for complex queries
		db.SetMaxOpenConns(2)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(10 * time.Minute)
	default:
		// Default configuration
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	}
}

// ReportConfigOptimized holds configuration for optimized report generation
type ReportConfigOptimized struct {
	DBFile           string
	OutputPath       string
	Format           string // "excel", "html", "console", "json"
	TopN             int    // For top largest files
	MinDuplicateSize int64  // Minimum file size to consider for duplicates
	EnableCache      bool   // Enable query result caching
	Verbose          bool   // Enable verbose logging
}

// ReportMetrics holds performance metrics for report generation
type ReportMetrics struct {
	TotalFiles      int64         `json:"totalFiles"`
	TotalSize       int64         `json:"totalSize"`
	DuplicateFiles  int64         `json:"duplicateFiles"`
	DuplicateSize   int64         `json:"duplicateSize"`
	GenerationTime  time.Duration `json:"generationTime"`
	QueriesExecuted int           `json:"queriesExecuted"`
	CacheHits       int           `json:"cacheHits"`
}

// ReportData holds all data needed for report generation
type ReportData struct {
	TopFiles    []FileInfoOptimized       `json:"topFiles"`
	Duplicates  []DuplicateGroupOptimized `json:"duplicates"`
	Summary     ReportSummary             `json:"summary"`
	Metrics     ReportMetrics             `json:"metrics"`
	GeneratedAt time.Time                 `json:"generatedAt"`
}

// FileInfo represents file information for reports
type FileInfoOptimized struct {
	ID     int64  `json:"id"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Mtime  string `json:"mtime"`
	Hash   string `json:"hash,omitempty"`
	LoaiTM string `json:"loaithumuc,omitempty"`
	ThuMuc string `json:"thumuc,omitempty"`
}

// DuplicateGroupOptimized represents a group of duplicate files
type DuplicateGroupOptimized struct {
	Hash      string              `json:"hash"`
	Size      int64               `json:"size"`
	Count     int                 `json:"count"`
	Files     []FileInfoOptimized `json:"files"`
	TotalSize int64               `json:"totalSize"`
}

// ReportSummary provides summary statistics
type ReportSummary struct {
	TotalFiles      int64 `json:"totalFiles"`
	TotalSize       int64 `json:"totalSize"`
	UniqueFiles     int64 `json:"uniqueFiles"`
	DuplicateFiles  int64 `json:"duplicateFiles"`
	WastedSpace     int64 `json:"wastedSpace"`
	AverageFileSize int64 `json:"averageFileSize"`
}

// QueryCache provides simple caching for query results
type QueryCache struct {
	data map[string]interface{}
	ttl  map[time.Time]string
}

// NewQueryCache creates a new query cache
func NewQueryCache() *QueryCache {
	return &QueryCache{
		data: make(map[string]interface{}),
		ttl:  make(map[time.Time]string),
	}
}

// Get retrieves cached data
func (qc *QueryCache) Get(key string) (interface{}, bool) {
	data, exists := qc.data[key]
	return data, exists
}

// Set stores data in cache
func (qc *QueryCache) Set(key string, value interface{}) {
	qc.data[key] = value
}

// OptimizedReporter generates reports with performance optimizations
type OptimizedReporter struct {
	logger  *logrus.Logger
	db      *sql.DB
	config  *ReportConfigOptimized
	cache   *QueryCache
	metrics *ReportMetrics
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewOptimizedReporter creates a new optimized reporter
func NewOptimizedReporter(config *ReportConfigOptimized) *OptimizedReporter {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	if config.Verbose {
		logger.SetLevel(logrus.DebugLevel)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)

	return &OptimizedReporter{
		logger:  logger,
		config:  config,
		cache:   NewQueryCache(),
		metrics: &ReportMetrics{},
		ctx:     ctx,
		cancel:  cancel,
	}
}

// generateReport generates the complete report
func (r *OptimizedReporter) generateReport() error {
	startTime := time.Now()
	r.metrics.GenerationTime = 0

	r.logger.WithFields(logrus.Fields{
		"dbFile": r.config.DBFile,
		"format": r.config.Format,
		"topN":   r.config.TopN,
	}).Info("Starting optimized report generation")

	// Connect to database
	db, err := openDBSQLite(r.config.DBFile)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()
	r.db = db

	// Configure database for optimal reporting
	configureDB(db, "report", 1)

	// Collect report data
	reportData, err := r.collectReportData()
	if err != nil {
		return fmt.Errorf("failed to collect report data: %w", err)
	}

	// Generate report in specified format
	switch r.config.Format {
	case "excel":
		err = r.generateExcelReport(reportData)
	case "html":
		err = r.generateHTMLReport(reportData)
	case "json":
		err = r.generateJSONReport(reportData)
	case "console":
		err = r.generateConsoleReport(reportData)
	default:
		return fmt.Errorf("unsupported report format: %s", r.config.Format)
	}

	if err != nil {
		return fmt.Errorf("failed to generate %s report: %w", r.config.Format, err)
	}

	r.metrics.GenerationTime = time.Since(startTime)

	r.logger.WithFields(logrus.Fields{
		"duration":        r.metrics.GenerationTime.Milliseconds(),
		"totalFiles":      r.metrics.TotalFiles,
		"duplicateFiles":  r.metrics.DuplicateFiles,
		"duplicateSize":   r.metrics.DuplicateSize,
		"queriesExecuted": r.metrics.QueriesExecuted,
		"cacheHits":       r.metrics.CacheHits,
	}).Info("Report generation completed successfully")

	return nil
}

// collectReportData collects all data needed for the report
func (r *OptimizedReporter) collectReportData() (*ReportData, error) {
	data := &ReportData{
		GeneratedAt: time.Now(),
	}

	// Collect top largest files
	topFiles, err := r.getTopLargestFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to get top largest files: %w", err)
	}
	data.TopFiles = topFiles

	// Collect duplicate files
	duplicates, err := r.getDuplicateFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to get duplicate files: %w", err)
	}
	data.Duplicates = duplicates

	// Generate summary
	summary, err := r.generateSummary()
	if err != nil {
		return nil, fmt.Errorf("failed to generate summary: %w", err)
	}
	data.Summary = summary

	// Update metrics
	r.metrics.TotalFiles = summary.TotalFiles
	r.metrics.DuplicateFiles = summary.DuplicateFiles
	r.metrics.DuplicateSize = summary.WastedSpace

	data.Metrics = *r.metrics

	return data, nil
}

// getTopLargestFiles retrieves top N largest files with optimized query
func (r *OptimizedReporter) getTopLargestFiles() ([]FileInfoOptimized, error) {
	cacheKey := fmt.Sprintf("top_files_%d", r.config.TopN)
	if cached, found := r.cache.Get(cacheKey); found {
		r.metrics.CacheHits++
		return cached.([]FileInfoOptimized), nil
	}

	query := `
		SELECT id, path, size, st_mtime, loaithumuc, thumuc
		FROM fs_files
		WHERE size > 0
		ORDER BY size DESC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(r.ctx, query, r.config.TopN)
	if err != nil {
		return nil, fmt.Errorf("failed to query top files: %w", err)
	}
	defer rows.Close()

	r.metrics.QueriesExecuted++

	var files []FileInfoOptimized
	for rows.Next() {
		var file FileInfoOptimized
		var mtime time.Time

		err := rows.Scan(&file.ID, &file.Path, &file.Size, &mtime, &file.LoaiTM, &file.ThuMuc)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file row: %w", err)
		}

		file.Mtime = mtime.Format("2006-01-02 15:04:05")
		files = append(files, file)
	}

	if r.config.EnableCache {
		r.cache.Set(cacheKey, files)
	}

	return files, nil
}

// getDuplicateFiles retrieves duplicate file groups with optimized query
func (r *OptimizedReporter) getDuplicateFiles() ([]DuplicateGroupOptimized, error) {
	cacheKey := "duplicate_files"
	if cached, found := r.cache.Get(cacheKey); found {
		r.metrics.CacheHits++
		return cached.([]DuplicateGroupOptimized), nil
	}

	query := `
		SELECT hash_value, size, COUNT(*) as count, GROUP_CONCAT(id)
		FROM fs_files
		WHERE hash_value IS NOT NULL
		  AND hash_value != ''
		  AND size >= ?
		GROUP BY hash_value, size
		HAVING count > 1
		ORDER BY size DESC
	`

	rows, err := r.db.QueryContext(r.ctx, query, r.config.MinDuplicateSize)
	if err != nil {
		return nil, fmt.Errorf("failed to query duplicate groups: %w", err)
	}
	defer rows.Close()

	r.metrics.QueriesExecuted++

	var groups []DuplicateGroupOptimized
	for rows.Next() {
		var group DuplicateGroupOptimized
		var ids string
		var count int

		err := rows.Scan(&group.Hash, &group.Size, &count, &ids)
		if err != nil {
			return nil, fmt.Errorf("failed to scan duplicate group: %w", err)
		}

		group.Count = count
		group.TotalSize = group.Size * int64(count)

		// Get file details for this group
		files, err := r.getFilesByIDs(ids)
		if err != nil {
			return nil, fmt.Errorf("failed to get files for duplicate group: %w", err)
		}

		group.Files = files
		groups = append(groups, group)
	}

	if r.config.EnableCache {
		r.cache.Set(cacheKey, groups)
	}

	return groups, nil
}

// getFilesByIDs retrieves files by comma-separated IDs
func (r *OptimizedReporter) getFilesByIDs(ids string) ([]FileInfoOptimized, error) {
	idList := strings.Split(ids, ",")
	placeholders := strings.Repeat("?,", len(idList))
	placeholders = placeholders[:len(placeholders)-1]

	query := fmt.Sprintf(`
		SELECT id, path, size, st_mtime, loaithumuc, thumuc
		FROM fs_files
		WHERE id IN (%s)
		ORDER BY path
	`, placeholders)

	args := make([]interface{}, len(idList))
	for i, idStr := range idList {
		args[i] = idStr
	}

	rows, err := r.db.QueryContext(r.ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query files by IDs: %w", err)
	}
	defer rows.Close()

	r.metrics.QueriesExecuted++

	var files []FileInfoOptimized
	for rows.Next() {
		var file FileInfoOptimized
		var mtime time.Time

		err := rows.Scan(&file.ID, &file.Path, &file.Size, &mtime, &file.LoaiTM, &file.ThuMuc)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file row: %w", err)
		}

		file.Mtime = mtime.Format("2006-01-02 15:04:05")
		files = append(files, file)
	}

	return files, nil
}

// generateSummary creates report summary statistics
func (r *OptimizedReporter) generateSummary() (ReportSummary, error) {
	cacheKey := "report_summary"
	if cached, found := r.cache.Get(cacheKey); found {
		r.metrics.CacheHits++
		return cached.(ReportSummary), nil
	}

	summary := ReportSummary{}

	// Get total files and size
	err := r.db.QueryRowContext(r.ctx, `
		SELECT COUNT(*), COALESCE(SUM(size), 0)
		FROM fs_files
	`).Scan(&summary.TotalFiles, &summary.TotalSize)
	if err != nil {
		return summary, fmt.Errorf("failed to get total statistics: %w", err)
	}
	r.metrics.QueriesExecuted++

	// Get unique files count
	err = r.db.QueryRowContext(r.ctx, `
		SELECT COUNT(DISTINCT hash_value)
		FROM fs_files
		WHERE hash_value IS NOT NULL AND hash_value != ''
	`).Scan(&summary.UniqueFiles)
	if err != nil {
		return summary, fmt.Errorf("failed to get unique files count: %w", err)
	}
	r.metrics.QueriesExecuted++

	// Calculate derived metrics
	summary.DuplicateFiles = summary.TotalFiles - summary.UniqueFiles
	summary.WastedSpace = 0 // Will be calculated from duplicates
	summary.AverageFileSize = 0
	if summary.TotalFiles > 0 {
		summary.AverageFileSize = summary.TotalSize / summary.TotalFiles
	}

	// Calculate wasted space from duplicates
	err = r.db.QueryRowContext(r.ctx, `
		SELECT COALESCE(SUM((COUNT(*) - 1) * size), 0)
		FROM fs_files
		WHERE hash_value IS NOT NULL AND hash_value != ''
		GROUP BY hash_value, size
		HAVING COUNT(*) > 1
	`).Scan(&summary.WastedSpace)
	if err != nil && err != sql.ErrNoRows {
		return summary, fmt.Errorf("failed to calculate wasted space: %w", err)
	}
	r.metrics.QueriesExecuted++

	if r.config.EnableCache {
		r.cache.Set(cacheKey, summary)
	}

	return summary, nil
}

// generateExcelReport creates an optimized Excel report
func (r *OptimizedReporter) generateExcelReport(data *ReportData) error {
	r.logger.Info("Generating optimized Excel report")

	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			r.logger.WithError(err).Error("Error closing Excel file")
		}
	}()

	// Create sheets
	sheets := map[string]string{
		"Summary":    "Summary",
		"Top Files":  "Top_Largest_Files",
		"Duplicates": "Duplicate_Files",
	}

	for sheetName, sheetTitle := range sheets {
		index, err := f.NewSheet(sheetTitle)
		if err != nil {
			return fmt.Errorf("failed to create sheet %s: %w", sheetName, err)
		}
		if sheetName == "Summary" {
			f.SetActiveSheet(index)
		}
	}

	// Add summary data
	if err := r.addSummaryToExcel(f, sheets["Summary"], data.Summary, data.Metrics); err != nil {
		return fmt.Errorf("failed to add summary to Excel: %w", err)
	}

	// Add top files data
	if err := r.addTopFilesToExcel(f, sheets["Top Files"], data.TopFiles); err != nil {
		return fmt.Errorf("failed to add top files to Excel: %w", err)
	}

	// Add duplicates data
	if err := r.addDuplicatesToExcel(f, sheets["Duplicates"], data.Duplicates); err != nil {
		return fmt.Errorf("failed to add duplicates to Excel: %w", err)
	}

	// Set default sheet to Summary
	if summaryIndex, err := f.GetSheetIndex(sheets["Summary"]); err == nil && summaryIndex >= 0 {
		f.SetActiveSheet(summaryIndex)
	}

	// Save file
	if err := f.SaveAs(r.config.OutputPath); err != nil {
		return fmt.Errorf("failed to save Excel file: %w", err)
	}

	r.logger.WithField("output", r.config.OutputPath).Info("Excel report generated successfully")
	return nil
}

// addSummaryToExcel adds summary statistics to Excel sheet
func (r *OptimizedReporter) addSummaryToExcel(f *excelize.File, sheetName string, summary ReportSummary, metrics ReportMetrics) error {
	headers := []string{"Metric", "Value"}
	data := [][]interface{}{
		{"Generated At", time.Now().Format("2006-01-02 15:04:05")},
		{"Total Files", summary.TotalFiles},
		{"Total Size", formatBytes(summary.TotalSize)},
		{"Unique Files", summary.UniqueFiles},
		{"Duplicate Files", summary.DuplicateFiles},
		{"Wasted Space", formatBytes(summary.WastedSpace)},
		{"Average File Size", formatBytes(summary.AverageFileSize)},
		{"Generation Time (ms)", metrics.GenerationTime.Milliseconds()},
		{"Queries Executed", metrics.QueriesExecuted},
		{"Cache Hits", metrics.CacheHits},
	}

	// Write headers
	for i, header := range headers {
		cell := fmt.Sprintf("%s1", string(rune('A'+i)))
		f.SetCellValue(sheetName, cell, header)
	}

	// Write data
	for i, row := range data {
		rowNum := i + 2
		for j, value := range row {
			cell := fmt.Sprintf("%s%d", string(rune('A'+j)), rowNum)
			f.SetCellValue(sheetName, cell, value)
		}
	}

	return nil
}

// addTopFilesToExcel adds top largest files to Excel sheet
func (r *OptimizedReporter) addTopFilesToExcel(f *excelize.File, sheetName string, files []FileInfoOptimized) error {
	headers := []string{"ID", "Path", "Size", "Modified", "Type", "Folder"}

	// Write headers
	for i, header := range headers {
		cell := fmt.Sprintf("%s1", string(rune('A'+i)))
		f.SetCellValue(sheetName, cell, header)
	}

	// Write data
	for i, file := range files {
		rowNum := i + 2
		data := []interface{}{
			file.ID,
			file.Path,
			file.Size,
			file.Mtime,
			file.LoaiTM,
			file.ThuMuc,
		}

		for j, value := range data {
			cell := fmt.Sprintf("%s%d", string(rune('A'+j)), rowNum)
			f.SetCellValue(sheetName, cell, value)
		}
	}

	return nil
}

// addDuplicatesToExcel adds duplicate file groups to Excel sheet
func (r *OptimizedReporter) addDuplicatesToExcel(f *excelize.File, sheetName string, duplicates []DuplicateGroupOptimized) error {
	headers := []string{"Hash", "Size", "Count", "Total Size", "Files"}

	// Write headers
	for i, header := range headers {
		cell := fmt.Sprintf("%s1", string(rune('A'+i)))
		f.SetCellValue(sheetName, cell, header)
	}

	// Write data
	rowNum := 2
	for _, group := range duplicates {
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowNum), group.Hash)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowNum), group.Size)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", rowNum), group.Count)
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", rowNum), group.TotalSize)

		// Combine file paths
		var filePaths []string
		for _, file := range group.Files {
			filePaths = append(filePaths, file.Path)
		}
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", rowNum), strings.Join(filePaths, "; "))

		rowNum++
	}

	return nil
}

// generateHTMLReport creates an optimized HTML report
func (r *OptimizedReporter) generateHTMLReport(data *ReportData) error {
	r.logger.Info("Generating optimized HTML report")

	htmlTemplate := `<!DOCTYPE html>
<html>
<head>
    <title>Filesystem Analysis Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .header { background-color: #f0f0f0; padding: 20px; border-radius: 5px; }
        .section { margin: 20px 0; padding: 15px; border: 1px solid #ddd; border-radius: 5px; }
        table { width: 100%; border-collapse: collapse; margin: 10px 0; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #f2f2f2; }
        .metric { display: inline-block; margin: 10px; padding: 10px; background-color: #e9f7ef; border-radius: 3px; }
    </style>
</head>
<body>
    <div class="header">
        <h1>Filesystem Analysis Report</h1>
        <p>Generated: {{.GeneratedAt.Format "2006-01-02 15:04:05"}}</p>
    </div>

    <div class="section">
        <h2>Summary</h2>
        <div class="metric">Total Files: {{.Summary.TotalFiles}}</div>
        <div class="metric">Total Size: {{formatBytes .Summary.TotalSize}}</div>
        <div class="metric">Unique Files: {{.Summary.UniqueFiles}}</div>
        <div class="metric">Duplicate Files: {{.Summary.DuplicateFiles}}</div>
        <div class="metric">Wasted Space: {{formatBytes .Summary.WastedSpace}}</div>
        <div class="metric">Generation Time: {{.Metrics.GenerationTime}}</div>
    </div>

    <div class="section">
        <h2>Top Largest Files</h2>
        <table>
            <tr><th>Path</th><th>Size</th><th>Modified</th></tr>
            {{range .TopFiles}}
            <tr>
                <td>{{.Path}}</td>
                <td>{{formatBytes .Size}}</td>
                <td>{{.Mtime}}</td>
            </tr>
            {{end}}
        </table>
    </div>

    <div class="section">
        <h2>Duplicate Files</h2>
        {{range .Duplicates}}
        <h3>Hash: {{.Hash}} ({{.Count}} files, {{formatBytes .TotalSize}} total)</h3>
        <table>
            <tr><th>Path</th><th>Size</th><th>Modified</th></tr>
            {{range .Files}}
            <tr>
                <td>{{.Path}}</td>
                <td>{{formatBytes .Size}}</td>
                <td>{{.Mtime}}</td>
            </tr>
            {{end}}
        </table>
        {{end}}
    </div>
</body>
</html>`

	tmpl, err := template.New("html").Funcs(template.FuncMap{
		"formatBytes": formatBytes,
	}).Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse HTML template: %w", err)
	}

	file, err := os.Create(r.config.OutputPath)
	if err != nil {
		return fmt.Errorf("failed to create HTML file: %w", err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("failed to execute HTML template: %w", err)
	}

	r.logger.WithField("output", r.config.OutputPath).Info("HTML report generated successfully")
	return nil
}

// generateJSONReport creates a JSON report
func (r *OptimizedReporter) generateJSONReport(data *ReportData) error {
	r.logger.Info("Generating JSON report")

	file, err := os.Create(r.config.OutputPath)
	if err != nil {
		return fmt.Errorf("failed to create JSON file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode JSON data: %w", err)
	}

	r.logger.WithField("output", r.config.OutputPath).Info("JSON report generated successfully")
	return nil
}

// generateConsoleReport creates a console report
func (r *OptimizedReporter) generateConsoleReport(data *ReportData) error {
	fmt.Printf("=== FILESYSTEM ANALYSIS REPORT ===\n")
	fmt.Printf("Generated: %s\n\n", data.GeneratedAt.Format("2006-01-02 15:04:05"))

	// Summary
	fmt.Printf("SUMMARY:\n")
	fmt.Printf("  Total Files:     %d\n", data.Summary.TotalFiles)
	fmt.Printf("  Total Size:      %s\n", formatBytes(data.Summary.TotalSize))
	fmt.Printf("  Unique Files:    %d\n", data.Summary.UniqueFiles)
	fmt.Printf("  Duplicate Files: %d\n", data.Summary.DuplicateFiles)
	fmt.Printf("  Wasted Space:    %s\n", formatBytes(data.Summary.WastedSpace))
	fmt.Printf("  Generation Time: %v\n\n", data.Metrics.GenerationTime)

	// Top files
	fmt.Printf("TOP %d LARGEST FILES:\n", len(data.TopFiles))
	for i, file := range data.TopFiles {
		fmt.Printf("%2d. %-50s %s\n", i+1, truncateString(file.Path, 50), formatBytes(file.Size))
	}
	fmt.Println()

	// Duplicates
	fmt.Printf("DUPLICATE FILES (%d groups):\n", len(data.Duplicates))
	for i, group := range data.Duplicates {
		fmt.Printf("%2d. Hash: %s\n", i+1, group.Hash[:12]+"...")
		fmt.Printf("    Size: %s, Count: %d, Total: %s\n",
			formatBytes(group.Size), group.Count, formatBytes(group.TotalSize))
		fmt.Printf("    Files:\n")
		for _, file := range group.Files {
			fmt.Printf("      - %s\n", truncateString(file.Path, 46))
		}
		fmt.Println()
	}

	return nil
}

// formatBytes formats bytes into human readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// truncateString truncates a string to specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// mainOptimized optimized main function
func mainOptimized() {
	// Parse command line arguments
	config := &ReportConfigOptimized{}
	flag.StringVar(&config.DBFile, "dbfile", "", "Path to the scan.db file")
	flag.StringVar(&config.Format, "format", "console", "Output format: excel, html, console, json")
	flag.StringVar(&config.OutputPath, "output", "", "Output path for report file")
	flag.IntVar(&config.TopN, "topn", 100, "Number of top largest files to report")
	flag.Int64Var(&config.MinDuplicateSize, "min-duplicate-size", 1024, "Minimum file size to consider for duplicates (bytes)")
	flag.BoolVar(&config.EnableCache, "cache", true, "Enable query result caching")
	flag.BoolVar(&config.Verbose, "verbose", false, "Enable verbose logging")
	flag.Parse()

	if config.DBFile == "" {
		fmt.Fprintln(os.Stderr, "Error: -dbfile flag is required.")
		os.Exit(1)
	}

	// Set default output path if not provided
	if config.OutputPath == "" {
		baseName := strings.TrimSuffix(filepath.Base(config.DBFile), filepath.Ext(config.DBFile))
		switch config.Format {
		case "excel":
			config.OutputPath = fmt.Sprintf("%s_optimized_report.xlsx", baseName)
		case "html":
			config.OutputPath = fmt.Sprintf("%s_optimized_report.html", baseName)
		case "json":
			config.OutputPath = fmt.Sprintf("%s_optimized_report.json", baseName)
		default:
			config.OutputPath = "" // Console output
		}
	}

	// Create optimized reporter and generate report
	reporter := NewOptimizedReporter(config)
	defer reporter.cancel()

	if err := reporter.generateReport(); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating report: %v\n", err)
		os.Exit(1)
	}
}

// configureDB for report phase
func configureDBReport(db *sql.DB) {
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(10 * time.Minute)

	// Optimize SQLite for reporting
	db.Exec("PRAGMA journal_mode = WAL")
	db.Exec("PRAGMA synchronous = NORMAL")
	db.Exec("PRAGMA cache_size = -128000") // 128MB cache for reporting
	db.Exec("PRAGMA temp_store = MEMORY")
	db.Exec("PRAGMA mmap_size = 536870912") // 512MB memory map
	db.Exec("PRAGMA query_only = 1")        // Read-only for reporting
}

// Main function for optimized reporter
func main() {
	// Use the existing main functionality but with optimized reporting
	dbFile := flag.String("dbfile", "", "Database file path")
	outputPath := flag.String("output", "", "Output file path")
	format := flag.String("format", "excel", "Report format (excel, html, console, json)")
	topN := flag.Int("topn", 100, "Number of top largest files to include")
	minSize := flag.Int64("minsize", 1024, "Minimum file size for duplicates")
	flag.Parse()

	config := &ReportConfigOptimized{
		DBFile:           *dbFile,
		OutputPath:       *outputPath,
		Format:           *format,
		TopN:             *topN,
		MinDuplicateSize: *minSize,
	}

	if config.DBFile == "" {
		fmt.Fprintln(os.Stderr, "Error: Database file path required")
		flag.Usage()
		os.Exit(1)
	}

	if config.OutputPath == "" {
		fmt.Fprintln(os.Stderr, "Error: Output path required")
		flag.Usage()
		os.Exit(1)
	}

	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	reporter := NewOptimizedReporter(config)
	if err := reporter.generateReport(); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating report: %v\n", err)
		os.Exit(1)
	}
}
