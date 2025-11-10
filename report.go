// report.go
//go:build reporter

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"sort" // Added for sorting duplicate groups
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	"github.com/xuri/excelize/v2" // For Excel output
)

// ReportConfig holds configuration for report generation
type ReportConfig struct {
	DBFile     string
	OutputPath string
	Format     string // "excel", "html", "console"
	TopN       int    // For top largest files
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Println("Go Reporter starting...")

	var cfg ReportConfig
	flag.StringVar(&cfg.DBFile, "dbfile", "", "Path to the scan.db file (e.g., ./output_scans/scan_....db)")
	flag.StringVar(&cfg.Format, "format", "console", "Output format: excel, html, console")
	flag.StringVar(&cfg.OutputPath, "output", "", "Output path for report file (e.g., report.xlsx or report.html)")
	flag.IntVar(&cfg.TopN, "topn", 100, "Number of top largest files to report")
	flag.Parse()

	if cfg.DBFile == "" {
		log.Fatal("Error: -dbfile flag is required.")
	}

	if cfg.OutputPath == "" {
		// Default output path based on format and dbfile
		baseName := strings.TrimSuffix(filepath.Base(cfg.DBFile), filepath.Ext(cfg.DBFile))
		switch cfg.Format {
		case "excel":
			cfg.OutputPath = fmt.Sprintf("%s_report.xlsx", baseName)
		case "html":
			cfg.OutputPath = fmt.Sprintf("%s_report.html", baseName)
		default:
			// For console, no output file
		}
	}

	db, err := openDBSQLite(cfg.DBFile) // Assuming openDBSQLite is in common_db.go
	if err != nil {
		log.Fatalf("Failed to open database %s: %v", cfg.DBFile, err)
	}
	defer db.Close()

	log.Printf("Generating report from DB: %s, Format: %s, Output: %s", cfg.DBFile, cfg.Format, cfg.OutputPath)

	switch cfg.Format {
	case "excel":
		err = generateExcelReport(db, &cfg)
	case "html":
		err = generateHtmlReport(db, &cfg)
	case "console":
		err = generateConsoleReport(db, &cfg)
	default:
		log.Fatalf("Unsupported report format: %s", cfg.Format)
	}

	if err != nil {
		log.Fatalf("Failed to generate report: %v", err)
	}

	log.Println("Report generation complete.")
}

// generateExcelReport generates an Excel report
func generateExcelReport(db *sql.DB, cfg *ReportConfig) error {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("Error closing Excel file: %v", err)
		}
	}()

	// --- Top Largest Files Sheet ---
	sheetNameTop := "Top Largest Files"
	indexTop, err := f.NewSheet(sheetNameTop)
	if err != nil {
		return fmt.Errorf("failed to create sheet %s: %w", sheetNameTop, err)
	}
	f.SetActiveSheet(indexTop)

	// Write headers
	headersTop := []string{"Rank", "Size (Bytes)", "Path", "Filename", "Modified Time", "Hash Value", "Type"}
	for i, header := range headersTop {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetNameTop, cell, header)
	}

	// Get data
	topFiles, err := getTopLargestFiles(db, cfg.TopN)
	if err != nil {
		return fmt.Errorf("failed to get top largest files for Excel: %w", err)
	}

	// Populate data
	for i, file := range topFiles {
		row := i + 2 // Start from second row
		f.SetCellValue(sheetNameTop, fmt.Sprintf("A%d", row), i+1)
		f.SetCellValue(sheetNameTop, fmt.Sprintf("B%d", row), file.Size)
		f.SetCellValue(sheetNameTop, fmt.Sprintf("C%d", row), file.Path)
		f.SetCellValue(sheetNameTop, fmt.Sprintf("D%d", row), file.Filename)
		f.SetCellValue(sheetNameTop, fmt.Sprintf("E%d", row), file.Mtime.Format(time.RFC3339))
		f.SetCellValue(sheetNameTop, fmt.Sprintf("F%d", row), file.HashValue)
		f.SetCellValue(sheetNameTop, fmt.Sprintf("G%d", row), file.LoaiThuMuc)
	}

	// --- Duplicate Files Sheet ---
	sheetNameDup := "Duplicate Files"
	indexDup, err := f.NewSheet(sheetNameDup)
	if err != nil {
		return fmt.Errorf("failed to create sheet %s: %w", sheetNameDup, err)
	}
	f.SetActiveSheet(indexDup)

	// Write headers
	headersDup := []string{"Hash Value", "Count", "File Path", "Filename", "Size (Bytes)", "Modified Time", "Type"}
	for i, header := range headersDup {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetNameDup, cell, header)
	}

	// Get data
	duplicateGroups, err := getDuplicateFiles(db)
	if err != nil {
		return fmt.Errorf("failed to get duplicate files for Excel: %w", err)
	}

	// Populate data
	row := 2 // Start from second row
	for _, group := range duplicateGroups {
		for _, file := range group.Files {
			f.SetCellValue(sheetNameDup, fmt.Sprintf("A%d", row), group.HashValue)
			f.SetCellValue(sheetNameDup, fmt.Sprintf("B%d", row), group.Count)
			f.SetCellValue(sheetNameDup, fmt.Sprintf("C%d", row), file.Path)
			f.SetCellValue(sheetNameDup, fmt.Sprintf("D%d", row), file.Filename)
			f.SetCellValue(sheetNameDup, fmt.Sprintf("E%d", row), file.Size)
			f.SetCellValue(sheetNameDup, fmt.Sprintf("F%d", row), file.Mtime.Format(time.RFC3339))
			f.SetCellValue(sheetNameDup, fmt.Sprintf("G%d", row), file.LoaiThuMuc)
			row++
		}
	}

	// Remove default "Sheet1" if it exists and is visible
	if f.GetSheetName(0) == "Sheet1" {
		visible, err := f.GetSheetVisible("Sheet1")
		if err != nil {
			log.Printf("Warning: failed to get visibility for Sheet1: %v", err)
		}
		if visible {
			if err := f.DeleteSheet("Sheet1"); err != nil {
				log.Printf("Warning: Failed to delete default Sheet1: %v", err)
			}
		}
	}

	// Save the Excel file
	if err := f.SaveAs(cfg.OutputPath); err != nil {
		return fmt.Errorf("failed to save Excel file %s: %w", cfg.OutputPath, err)
	}

	log.Printf("Excel report saved to %s", cfg.OutputPath)
	return nil
}

// generateHtmlReport generates an HTML report
func generateHtmlReport(db *sql.DB, cfg *ReportConfig) error {
	file, err := os.Create(cfg.OutputPath)
	if err != nil {
		return fmt.Errorf("failed to create HTML file %s: %w", cfg.OutputPath, err)
	}
	defer file.Close()

	writer := file

	// Write HTML header
	fmt.Fprintf(writer, `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>File Scan Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background-color: #f4f4f4; color: #333; }
        h1, h2 { color: #0056b3; }
        table { width: 100%%%%; border-collapse: collapse; margin-bottom: 20px; background-color: #fff; box-shadow: 0 0 10px rgba(0, 0, 0, 0.1); }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #007bff; color: white; }
        tr:nth-child(even) { background-color: #f2f2f2; }
        tr:hover { background-color: #ddd; }
        .section { margin-bottom: 40px; }
        .hash-group { background-color: #e9ecef; font-weight: bold; }
    </style>
</head>
<body>
    <h1>File Scan Report</h1>
    <p>Generated on: %s</p>

    <div class="section">
        <h2>Top %d Largest Files</h2>
        <table>
            <thead>
                <tr>
                    <th>Rank</th>
                    <th>Size (Bytes)</th>
                    <th>Path</th>
                    <th>Filename</th>
                    <th>Modified Time</th>
                    <th>Hash Value</th>
                    <th>Type</th>
                </tr>
            </thead>
            <tbody>
`, time.Now().Format("2006-01-02 15:04:05"), cfg.TopN)

	// --- Top Largest Files Table ---
	topFiles, err := getTopLargestFiles(db, cfg.TopN)
	if err != nil {
		return fmt.Errorf("failed to get top largest files for HTML: %w", err)
	}
	for i, file := range topFiles {
		fmt.Fprintf(writer, `                <tr>
                    <td>%d</td>
                    <td>%d</td>
                    <td>%s</td>
                    <td>%s</td>
                    <td>%s</td>
                    <td>%s</td>
                    <td>%s</td>
                </tr>
`, i+1, file.Size, htmlEscape(file.Path), htmlEscape(file.Filename), file.Mtime.Format(time.RFC3339), file.HashValue, htmlEscape(file.LoaiThuMuc))
	}
	fmt.Fprintf(writer, `            </tbody>
        </table>
    </div>

    <div class="section">
        <h2>Duplicate Files</h2>
        <table>
            <thead>
                <tr>
                    <th>Hash Value</th>
                    <th>Count</th>
                    <th>File Path</th>
                    <th>Filename</th>
                    <th>Size (Bytes)</th>
                    <th>Modified Time</th>
                    <th>Type</th>
                </tr>
            </thead>
            <tbody>
`)

	// --- Duplicate Files Table ---
	duplicateGroups, err := getDuplicateFiles(db)
	if err != nil {
		return fmt.Errorf("failed to get duplicate files for HTML: %w", err)
	}
	for _, group := range duplicateGroups {
		fmt.Fprintf(writer, `                <tr class="hash-group">
                    <td colspan="7">Hash: %s (Count: %d)</td>
                </tr>
`, htmlEscape(group.HashValue), group.Count)
		for _, file := range group.Files {
			fmt.Fprintf(writer, `                <tr>
                    <td></td>
                    <td></td>
                    <td>%s</td>
                    <td>%s</td>
                    <td>%d</td>
                    <td>%s</td>
                    <td>%s</td>
                </tr>
`, htmlEscape(file.Path), htmlEscape(file.Filename), file.Size, file.Mtime.Format(time.RFC3339), htmlEscape(file.LoaiThuMuc))
		}
	}
	fmt.Fprintf(writer, `            </tbody>
        </table>
    </div>

</body>
</html>
`)

	log.Printf("HTML report saved to %s", cfg.OutputPath)
	return nil
}

// htmlEscape escapes strings for HTML output
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// generateConsoleReport generates a report to the console
func generateConsoleReport(db *sql.DB, cfg *ReportConfig) error {
	fmt.Println("--- Top Largest Files ---")
	topFiles, err := getTopLargestFiles(db, cfg.TopN)
	if err != nil {
		return fmt.Errorf("failed to get top largest files: %w", err)
	}
	for i, file := range topFiles {
		fmt.Printf("%d. Size: %-10d Path: %s", i+1, file.Size, file.Path)
	}
	fmt.Println()
	fmt.Println("--- Duplicate Files ---")
	duplicateGroups, err := getDuplicateFiles(db)
	if err != nil {
		return fmt.Errorf("failed to get duplicate files: %w", err)
	}
	for _, group := range duplicateGroups {
		fmt.Printf("Hash: %s (Count: %d)", group.HashValue, group.Count)
		for _, file := range group.Files {
			fmt.Printf("  - Size: %-10d Path: %s", file.Size, file.Path)
		}
		fmt.Println()
	}
	return nil
}

// getTopLargestFiles fetches the top N largest files from the database
func getTopLargestFiles(db *sql.DB, topN int) ([]FileInfo, error) {
	rows, err := db.Query(`
		SELECT id, path, filename, size, st_mtime, hash_value, loaithumuc
		FROM fs_files
		ORDER BY size DESC
		LIMIT ?
	`, topN)
	if err != nil {
		return nil, fmt.Errorf("query top largest files failed: %w", err)
	}
	defer rows.Close()

	var files []FileInfo
	for rows.Next() {
		var file FileInfo
		var hash sql.NullString
		if err := rows.Scan(&file.ID, &file.Path, &file.Filename, &file.Size, &file.Mtime, &hash, &file.LoaiThuMuc); err != nil {
			return nil, fmt.Errorf("scan top largest file row failed: %w", err)
		}
		if hash.Valid {
			file.HashValue = hash.String
		}
		files = append(files, file)
	}
	return files, nil
}

// getDuplicateFiles fetches groups of duplicate files from the database
func getDuplicateFiles(db *sql.DB) ([]DuplicateGroup, error) {
	rows, err := db.Query(`
		SELECT f.id, f.path, f.filename, f.size, f.st_mtime, f.hash_value, f.loaithumuc
		FROM fs_files f
		JOIN (
			SELECT hash_value
			FROM fs_files
			WHERE hash_value IS NOT NULL AND hash_value != ''
			GROUP BY hash_value
			HAVING COUNT(*) > 1
		) AS duplicates ON f.hash_value = duplicates.hash_value
		ORDER BY f.hash_value, f.size DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query duplicate files failed: %w", err)
	}
	defer rows.Close()

	duplicateMap := make(map[string]*DuplicateGroup)
	for rows.Next() {
		var file FileInfo
		var hash sql.NullString
		if err := rows.Scan(&file.ID, &file.Path, &file.Filename, &file.Size, &file.Mtime, &hash, &file.LoaiThuMuc); err != nil {
			return nil, fmt.Errorf("scan duplicate file row failed: %w", err)
		}
		if hash.Valid {
			file.HashValue = hash.String
		} else {
			continue // Skip files without hash_value
		}

		group, ok := duplicateMap[file.HashValue]
		if !ok {
			group = &DuplicateGroup{
				HashValue: file.HashValue,
				Count:     0, // Will be updated later
				Files:     []FileInfo{},
			}
			duplicateMap[file.HashValue] = group
		}
		group.Files = append(group.Files, file)
	}

	var duplicateGroups []DuplicateGroup
	for _, group := range duplicateMap {
		group.Count = len(group.Files)
		duplicateGroups = append(duplicateGroups, *group)
	}

	// Sort groups by hash value for consistent output
	sort.Slice(duplicateGroups, func(i, j int) bool {
		return duplicateGroups[i].HashValue < duplicateGroups[j].HashValue
	})

	return duplicateGroups, nil
}

// FileInfo struct to hold file details for reports
type FileInfo struct {
	ID         int64
	Path       string
	Filename   string
	Size       int64
	Mtime      time.Time
	HashValue  string
	LoaiThuMuc string
}

// DuplicateGroup struct to hold info about duplicate files
type DuplicateGroup struct {
	HashValue string
	Count     int
	Files     []FileInfo
}
