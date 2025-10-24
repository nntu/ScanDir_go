// common_config.go
//go:build scanner || deleter || reporter

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-ini/ini"
)

// loadConfig tải cấu hình từ config.ini
func loadConfig(path string) (*Config, error) {
	cfg, err := ini.Load(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	secOut := cfg.Section("output")
	outDir := secOut.Key("output_dir").MustString("./output_scans")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create output dir %s: %w", outDir, err)
	}

	secScan := cfg.Section("scan")
	batch := secScan.Key("BATCH_SIZE").MustInt(5000)
	workers := secScan.Key("MAX_WORKERS").MustInt(4)
	excl := secScan.Key("EXCLUDE_DIRS").MustString(".git,.streams,@Recently-Snapshot,@Recycle,COREBanking")
	exclude := map[string]struct{}{}
	for _, n := range strings.Split(excl, ",") {
		n = strings.TrimSpace(n)
		if n != "" {
			exclude[n] = struct{}{}
		}
	}

	secPaths := cfg.Section("paths")
	paths := [][2]string{}
	for _, k := range secPaths.Keys() {
		v := k.Value()
		if p, t, ok := strings.Cut(v, ":"); ok {
			paths = append(paths, [2]string{strings.TrimSpace(p), strings.TrimSpace(t)})
		} else if v != "" {
			paths = append(paths, [2]string{v, filepath.Base(v)})
		}
	}

	return &Config{
		OutputDir:  outDir,
		BatchSize:  batch,
		MaxWorkers: workers,
		Exclude:    exclude,
		Paths:      paths,
	}, nil
}

// topFolder (dùng chung)
func topFolder(path string, depth int) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > depth {
		return parts[depth]
	}
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}