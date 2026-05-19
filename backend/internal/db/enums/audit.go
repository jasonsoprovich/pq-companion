package enums

import (
	"database/sql"
	"fmt"
	"sort"
)

// AuditDef describes how to validate one enum against a live quarm.db:
// the canonical set of codes we know how to label, and an Extract
// function that returns all distinct values for this enum present in
// the database.
//
// Each enum file in this package exposes an AuditDef as a package-level
// variable; Defs() aggregates them so the enum-audit CLI can iterate.
type AuditDef struct {
	Name       string
	KnownCodes map[int]struct{}
	Extract    func(*sql.DB) ([]int, error)
}

// AuditFinding is the per-enum result of RunAudit.
type AuditFinding struct {
	Name     string
	Known    int   // size of KnownCodes
	Observed int   // count of distinct codes seen in DB
	Unknown  []int // observed codes that aren't in KnownCodes (sorted asc)
}

// Defs returns every enum's AuditDef in a stable order. Add new enums
// to this list as they are migrated into the package.
func Defs() []AuditDef {
	return []AuditDef{
		TradeskillsAudit,
		SpecialAbilitiesAudit,
		ItemTypesAudit,
	}
}

// RunAudit executes every AuditDef against the provided database and
// returns one AuditFinding per enum.
func RunAudit(db *sql.DB) ([]AuditFinding, error) {
	defs := Defs()
	findings := make([]AuditFinding, 0, len(defs))
	for _, def := range defs {
		observed, err := def.Extract(db)
		if err != nil {
			return nil, fmt.Errorf("audit %s: %w", def.Name, err)
		}
		var unknown []int
		for _, code := range observed {
			if _, ok := def.KnownCodes[code]; !ok {
				unknown = append(unknown, code)
			}
		}
		sort.Ints(unknown)
		findings = append(findings, AuditFinding{
			Name:     def.Name,
			Known:    len(def.KnownCodes),
			Observed: len(observed),
			Unknown:  unknown,
		})
	}
	return findings, nil
}

// keysAsSet returns a set view of the integer keys of m. Used by per-
// enum AuditDef declarations to share their canonical code list with
// the validator without duplicating it.
func keysAsSet[V any](m map[int]V) map[int]struct{} {
	out := make(map[int]struct{}, len(m))
	for k := range m {
		out[k] = struct{}{}
	}
	return out
}
