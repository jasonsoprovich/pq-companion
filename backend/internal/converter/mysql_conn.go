package converter

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

// ConvertFromMySQL connects to a live MySQL database and converts all tables to SQLite.
func ConvertFromMySQL(ctx context.Context, cfg Config, dsn string, sqliteDB *sql.DB) error {
	if err := configureSQLite(sqliteDB); err != nil {
		return fmt.Errorf("configure sqlite: %w", err)
	}

	mysqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open mysql: %w", err)
	}
	defer mysqlDB.Close()

	if err := mysqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping mysql: %w", err)
	}
	cfg.log().Info("connected to MySQL")

	tables, err := listTables(ctx, mysqlDB)
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}
	cfg.log().Info("found tables", "count", len(tables))

	// First pass: create all schemas
	for _, table := range tables {
		if err := createTableFromMySQL(ctx, cfg, mysqlDB, sqliteDB, table); err != nil {
			return fmt.Errorf("create table %s: %w", table, err)
		}
	}

	// Second pass: copy all data
	totalRows := 0
	for i, table := range tables {
		n, err := copyTableData(ctx, cfg, mysqlDB, sqliteDB, table)
		if err != nil {
			return fmt.Errorf("copy data %s: %w", table, err)
		}
		totalRows += n
		cfg.log().Info("copied table",
			"table", table,
			"rows", n,
			"progress", fmt.Sprintf("%d/%d", i+1, len(tables)))
	}

	cfg.log().Info("MySQL conversion complete",
		"tables", len(tables),
		"total_rows", totalRows)
	return nil
}

// listTables returns all table names in the current MySQL database.
func listTables(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, "SHOW TABLES")
	if err != nil {
		return nil, fmt.Errorf("SHOW TABLES: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// createTableFromMySQL creates a SQLite table from a MySQL SHOW CREATE TABLE result.
func createTableFromMySQL(ctx context.Context, cfg Config, mysqlDB, sqliteDB *sql.DB, table string) error {
	row := mysqlDB.QueryRowContext(ctx, "SHOW CREATE TABLE `"+table+"`")

	var tableName, createStmt string
	if err := row.Scan(&tableName, &createStmt); err != nil {
		return fmt.Errorf("SHOW CREATE TABLE %s: %w", table, err)
	}

	return execCreateTable(cfg, createStmt, sqliteDB)
}

// copyTableData copies all rows from a MySQL table to the SQLite table.
func copyTableData(ctx context.Context, cfg Config, mysqlDB, sqliteDB *sql.DB, table string) (int, error) {
	rows, err := mysqlDB.QueryContext(ctx, "SELECT * FROM `"+table+"`")
	if err != nil {
		return 0, fmt.Errorf("SELECT: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("columns: %w", err)
	}
	if len(cols) == 0 {
		return 0, nil
	}

	tx, err := sqliteDB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}

	// Batch size: keep total parameters within SQLite's limit.
	const maxParams = 9999
	batchSize := maxParams / len(cols)
	if batchSize < 1 {
		batchSize = 1
	}
	if batchSize > 500 {
		batchSize = 500
	}

	rowPlaceholder := "(" + strings.Repeat("?,", len(cols)-1) + "?)"

	// Reusable scan destinations
	dest := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range dest {
		ptrs[i] = &dest[i]
	}

	// Accumulate rows into a batch, then flush with a multi-row INSERT.
	batch := make([][]interface{}, 0, batchSize)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		placeholders := make([]string, len(batch))
		for i := range batch {
			placeholders[i] = rowPlaceholder
		}
		batchQuery := "INSERT OR REPLACE INTO " + quoteIdent(table) +
			" VALUES " + strings.Join(placeholders, ",")

		args := make([]interface{}, len(batch)*len(cols))
		for i, row := range batch {
			copy(args[i*len(cols):], row)
		}

		if _, err := tx.ExecContext(ctx, batchQuery, args...); err != nil {
			return fmt.Errorf("batch insert: %w", err)
		}
		batch = batch[:0]
		return nil
	}

	inserted := 0

	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("scan row: %w", err)
		}

		row := make([]interface{}, len(cols))
		copy(row, dest)
		batch = append(batch, row)
		inserted++

		if len(batch) >= batchSize {
			if err := flush(); err != nil {
				tx.Rollback()
				return 0, err
			}
		}
	}
	if err := rows.Err(); err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("rows: %w", err)
	}

	if err := flush(); err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("final commit: %w", err)
	}

	return inserted, nil
}
