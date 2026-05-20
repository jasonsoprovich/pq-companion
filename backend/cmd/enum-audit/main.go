// enum-audit scans a quarm.db SQLite file and reports any raw numeric
// codes that the backend's enums package doesn't recognize. Run it
// after every Project Quarm DB refresh — if it exits 0 with no
// findings, the dump is safe to ship; if it flags unknown codes, those
// rows in docs/enum-audit.md need updating before the labels appear in
// the UI.
//
// With -samples, the tool emits a markdown report instead: for every
// enum code observed in the DB, it lists the current label and a few
// example rows (spell/item/NPC names) so the labels can be visually
// cross-checked against pqdi.cc and the EQMacEmu source. Useful for
// catching label drift like the "Single (Pet)" / "PB AE" mix-up where
// the code itself is known but the label is wrong.
//
// Usage (from the backend/ directory, where go.mod lives):
//
//	go run ./cmd/enum-audit                 # defaults to ./data/quarm.db
//	go run ./cmd/enum-audit -db /path/to/quarm.db
//	go run ./cmd/enum-audit -samples        # markdown report to stdout
//	go run ./cmd/enum-audit -samples -out docs/enum-samples.md
//
// Exit codes:
//
//	0 — every observed code is known (or -samples report produced)
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
	samples := flag.Bool("samples", false, "emit a markdown sample report for visual verification against pqdi")
	sampleLimit := flag.Int("sample-limit", 3, "examples per enum code in the sample report")
	outPath := flag.String("out", "", "write report to this file instead of stdout (samples mode only)")
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

	if *samples {
		reports, err := enums.BuildSampleReport(conn, *sampleLimit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sample report failed: %v\n", err)
			os.Exit(2)
		}
		md := renderSamplesMarkdown(*dbPath, reports)
		if *outPath == "" {
			fmt.Print(md)
		} else {
			if err := os.WriteFile(*outPath, []byte(md), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "write %s: %v\n", *outPath, err)
				os.Exit(2)
			}
			fmt.Fprintf(os.Stderr, "wrote %s (%d enums)\n", *outPath, len(reports))
		}
		return
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

// renderSamplesMarkdown formats sample reports as a single markdown
// document: one H2 section per enum, with each observed code rendered
// as `code → label — examples` so the human reviewer can scan against
// pqdi or the EQMacEmu source without flipping tools.
func renderSamplesMarkdown(dbPath string, reports []enums.SampleReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Enum sample report\n\n")
	fmt.Fprintf(&b, "Generated from `%s`. Each enum lists every code observed in the\n", dbPath)
	fmt.Fprintf(&b, "live dump, the label PQ Companion currently displays, and up to a\n")
	fmt.Fprintf(&b, "few example rows. Scan each section against pqdi.cc and the cited\n")
	fmt.Fprintf(&b, "upstream source; mismatches indicate label drift (the code is\n")
	fmt.Fprintf(&b, "known but the name is wrong — the kind of bug the unknown-code\n")
	fmt.Fprintf(&b, "audit cannot catch on its own).\n\n")

	for _, r := range reports {
		fmt.Fprintf(&b, "## %s\n\n", r.Name)
		if r.Source != "" {
			fmt.Fprintf(&b, "**Source:** %s\n\n", r.Source)
		}
		if len(r.Codes) == 0 {
			fmt.Fprintf(&b, "_No codes observed in the DB._\n\n")
			continue
		}
		fmt.Fprintf(&b, "| Code | Label | Examples |\n")
		fmt.Fprintf(&b, "|-----:|-------|----------|\n")
		for _, c := range r.Codes {
			label := c.Label
			if label == "" {
				label = "**(unknown — investigate)**"
			}
			examples := "_no sample available_"
			if len(c.Samples) > 0 {
				parts := make([]string, 0, len(c.Samples))
				for _, s := range c.Samples {
					name := s.Name
					if name == "" {
						name = "(unnamed)"
					}
					parts = append(parts, fmt.Sprintf("`%d` %s", s.ID, name))
				}
				examples = strings.Join(parts, " · ")
			}
			fmt.Fprintf(&b, "| %d | %s | %s |\n", c.Code, label, examples)
		}
		fmt.Fprintln(&b)
	}
	return b.String()
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
