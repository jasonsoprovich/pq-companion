// Command dbconvert converts a MySQL database to a SQLite file for PQ Companion.
//
// Usage:
//
//	dbconvert --from-dump [--sql-dir ./sql] [--output ./backend/data/quarm.db]
//	dbconvert --from-mysql [--mysql-dsn root:pass@tcp(host:port)/db] [--output ...]
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/converter"
	_ "modernc.org/sqlite"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fromDump := flag.Bool("from-dump", false, "Convert from MySQL .sql dump files")
	fromMySQL := flag.Bool("from-mysql", false, "Convert from live MySQL connection")
	sqlDir := flag.String("sql-dir", "", "Directory containing .sql dump files (default: auto-detect)")
	sqlFiles := flag.String("sql-files", "", "Comma-separated list of specific .sql files to process")
	mysqlDSN := flag.String("mysql-dsn", "root:quarmbuddy@tcp(localhost:3306)/quarm", "MySQL DSN for --from-mysql mode")
	output := flag.String("output", "", "Output SQLite database path (default: backend/data/quarm.db)")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	validate := flag.Bool("validate", true, "Run validation after conversion (row counts, FK integrity, spot checks)")
	validateOnly := flag.Bool("validate-only", false, "Skip conversion; only validate the existing output database")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `dbconvert — MySQL to SQLite converter for PQ Companion

Usage:
  dbconvert --from-dump [flags]
  dbconvert --from-mysql [flags]

Flags:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  # Convert from SQL dump files in ./sql/ directory
  dbconvert --from-dump

  # Convert from SQL dump files in a specific directory
  dbconvert --from-dump --sql-dir /path/to/sql

  # Convert from specific SQL files
  dbconvert --from-dump --sql-files quarm.sql,player_tables.sql

  # Convert from live MySQL (requires Docker to be running)
  dbconvert --from-mysql

  # Convert from MySQL with custom DSN and output path
  dbconvert --from-mysql --mysql-dsn "user:pass@tcp(host:3306)/db" --output /path/to/out.db

  # Skip validation (e.g. for a partial test run)
  dbconvert --from-dump --validate=false

  # Validate an existing database without re-running the conversion
  dbconvert --validate-only --output /path/to/quarm.db
`)
	}

	flag.Parse()

	if *validateOnly {
		if *fromDump || *fromMySQL {
			return fmt.Errorf("--validate-only cannot be combined with --from-dump/--from-mysql")
		}
	} else {
		if !*fromDump && !*fromMySQL {
			flag.Usage()
			return fmt.Errorf("must specify --from-dump, --from-mysql, or --validate-only")
		}
		if *fromDump && *fromMySQL {
			return fmt.Errorf("cannot specify both --from-dump and --from-mysql")
		}
	}

	// Determine output path
	outPath := *output
	if outPath == "" {
		// Auto-detect: look for backend/data/ relative to CWD or script location
		outPath = findOutputPath()
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	// Set up logging
	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	cfg := converter.Config{
		Verbose: *verbose,
		Logger:  logger,
	}

	// Open SQLite database
	logger.Info("opening SQLite database", "path", outPath)
	db, err := sql.Open("sqlite", outPath)
	if err != nil {
		return fmt.Errorf("open sqlite %s: %w", outPath, err)
	}
	defer db.Close()

	// SQLite performs best with a single writer connection
	db.SetMaxOpenConns(1)

	ctx := context.Background()

	switch {
	case *validateOnly:
		logger.Info("validating existing database", "path", outPath)
	case *fromDump:
		files, err := resolveDumpFiles(*sqlDir, *sqlFiles)
		if err != nil {
			return fmt.Errorf("resolve dump files: %w", err)
		}
		logger.Info("converting from dump files", "files", files, "output", outPath)
		if err := converter.ConvertFromDump(ctx, cfg, files, db); err != nil {
			return err
		}
	case *fromMySQL:
		logger.Info("converting from MySQL", "dsn", redactDSN(*mysqlDSN), "output", outPath)
		if err := converter.ConvertFromMySQL(ctx, cfg, *mysqlDSN, db); err != nil {
			return err
		}
	}

	if !*validate && !*validateOnly {
		return nil
	}

	logger.Info("running validation")
	rep, err := converter.Validate(ctx, cfg, db)
	if err != nil {
		return fmt.Errorf("validate: %w", err)
	}
	if rep.Errors > 0 {
		return fmt.Errorf("validation failed: %d error(s), %d warning(s)", rep.Errors, rep.Warnings)
	}
	logger.Info("validation passed", "warnings", rep.Warnings)
	return nil
}

// resolveDumpFiles returns the list of SQL dump files to process.
// Priority: explicit --sql-files > --sql-dir > auto-detect.
func resolveDumpFiles(sqlDir, sqlFiles string) ([]string, error) {
	if sqlFiles != "" {
		parts := strings.Split(sqlFiles, ",")
		var files []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				files = append(files, p)
			}
		}
		return files, nil
	}

	dir := sqlDir
	if dir == "" {
		dir = findSQLDir()
	}
	if dir == "" {
		return nil, fmt.Errorf("cannot find SQL directory; use --sql-dir or --sql-files")
	}

	return discoverDumpFiles(dir)
}

// discoverDumpFiles finds SQL dump files in dir, excluding helper files.
// Returns files in a deterministic order: main dump first, then others.
func discoverDumpFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}

	// Files to skip
	skip := map[string]bool{
		"drop_system.sql":  true,
		"example_queries.sql": true,
	}

	var main []string
	var others []string

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		if skip[e.Name()] {
			continue
		}
		path := filepath.Join(dir, e.Name())
		name := strings.ToLower(e.Name())
		// quarm_*.sql is the main dump; process it first
		if strings.HasPrefix(name, "quarm") {
			main = append(main, path)
		} else {
			others = append(others, path)
		}
	}

	sort.Strings(others)

	return append(main, others...), nil
}

// findSQLDir tries to locate the sql/ directory relative to common working directories.
func findSQLDir() string {
	candidates := []string{
		"sql",
		"../sql",
		"../../sql",
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

// findOutputPath returns a suitable default output path.
func findOutputPath() string {
	candidates := []string{
		"backend/data/quarm.db",
		"../data/quarm.db",
		"data/quarm.db",
	}
	for _, c := range candidates {
		dir := filepath.Dir(c)
		if _, err := os.Stat(dir); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	// fallback
	abs, _ := filepath.Abs("quarm.db")
	return abs
}

// redactDSN hides the password from a MySQL DSN for logging.
func redactDSN(dsn string) string {
	// Format: user:pass@tcp(host)/db
	at := strings.LastIndex(dsn, "@")
	if at < 0 {
		return dsn
	}
	userInfo := dsn[:at]
	rest := dsn[at:]
	colon := strings.Index(userInfo, ":")
	if colon < 0 {
		return dsn
	}
	return userInfo[:colon+1] + "***" + rest
}
