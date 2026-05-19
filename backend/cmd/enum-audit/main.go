// enum-audit scans a quarm.db SQLite file and reports any raw numeric
// codes that the backend's enums package doesn't recognize. Run it
// after every Project Quarm DB refresh — if it exits 0 with no
// findings, the dump is safe to ship; if it flags unknown codes, those
// rows in docs/enum-audit.md need updating before the labels appear in
// the UI.
//
// Usage (from the backend/ directory, where go.mod lives):
//
//	go run ./cmd/enum-audit                 # defaults to ./data/quarm.db
//	go run ./cmd/enum-audit -db /path/to/quarm.db
//
// Exit codes:
//
//	0 — every observed code is known
//	1 — at least one enum reported unknown codes (see stdout)
//	2 — could not open/query the database
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db/enums"
	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "data/quarm.db", "path to quarm.db (relative to backend/ by default)")
	flag.Parse()

	conn, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", *dbPath, err)
		os.Exit(2)
	}
	defer conn.Close()
	if err := conn.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "ping %s: %v\n", *dbPath, err)
		os.Exit(2)
	}

	findings, err := enums.RunAudit(conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audit failed: %v\n", err)
		os.Exit(2)
	}

	printReport(*dbPath, findings)

	for _, f := range findings {
		if len(f.Unknown) > 0 {
			os.Exit(1)
		}
	}
}

func printReport(dbPath string, findings []enums.AuditFinding) {
	fmt.Printf("Enum audit — %s\n", dbPath)
	fmt.Println(strings.Repeat("=", 60))

	maxName := len("Enum")
	for _, f := range findings {
		if len(f.Name) > maxName {
			maxName = len(f.Name)
		}
	}

	header := fmt.Sprintf("%-*s  %8s  %8s  %8s   %s", maxName, "Enum", "Known", "Observed", "Unknown", "Status")
	fmt.Println(header)
	fmt.Println(strings.Repeat("-", len(header)))

	totalUnknown := 0
	for _, f := range findings {
		status := "OK"
		if len(f.Unknown) > 0 {
			status = "FAIL"
			totalUnknown += len(f.Unknown)
		}
		fmt.Printf("%-*s  %8d  %8d  %8d   %s\n", maxName, f.Name, f.Known, f.Observed, len(f.Unknown), status)
		for _, code := range f.Unknown {
			fmt.Printf("%-*s    └─ unknown code: %d\n", maxName, "", code)
		}
	}

	fmt.Println()
	if totalUnknown == 0 {
		fmt.Println("All clean.")
	} else {
		fmt.Printf("Found %d unknown codes across %d enum(s). Update backend/internal/db/enums/ and docs/enum-audit.md before shipping.\n", totalUnknown, countFailing(findings))
	}
}

func countFailing(findings []enums.AuditFinding) int {
	n := 0
	for _, f := range findings {
		if len(f.Unknown) > 0 {
			n++
		}
	}
	return n
}
