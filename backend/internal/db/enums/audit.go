package enums

import (
	"database/sql"
	"fmt"
	"sort"
)

// AuditDef describes how to validate one enum against a live quarm.db:
// the canonical set of codes we know how to label, an Extract function
// that returns all distinct values for this enum present in the
// database, and (optionally) a Sample function that returns a handful of
// example rows for a given code so the label can be spot-checked
// against pqdi.cc.
//
// Each enum file in this package exposes an AuditDef as a package-level
// variable; Defs() aggregates them so the enum-audit CLI can iterate.
type AuditDef struct {
	Name       string
	KnownCodes map[int]struct{}
	Extract    func(*sql.DB) ([]int, error)
	// Sample is optional. When non-nil, BuildSampleReport calls it for
	// every observed code to surface example DB rows for visual
	// verification against canonical sources (pqdi, alla, EQMacEmu src).
	// Bitmask / pure-label enums leave this nil.
	Sample func(db *sql.DB, code, limit int) ([]SampleRow, error)
}

// SampleRow is one example DB row associated with an enum code — used
// to give a human spot-checker something concrete to verify against
// (e.g. "code 4 → 'PB AE' → [Earthquake, Chords of Dissonance]").
type SampleRow struct {
	ID   int
	Name string
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
		ContainerTypesAudit,
		SpecialAbilitiesAudit,
		ItemTypesAudit,
		NPCClassesAudit,
		NPCRacesAudit,
		ItemSlotBitsAudit,
		ItemClassBitsAudit,
		ItemRaceBitsAudit,
		BaneBodiesAudit,
		BaneRacesAudit,
		ZoneExpansionsAudit,
		ZoneTypesAudit,
		NPCBodyTypesAudit,
		SpellEffectsAudit,
		SpellResistsAudit,
		SpellTargetsAudit,
		SpellSkillsAudit,
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

// SampleReport is the per-enum payload of BuildSampleReport: every
// observed code paired with its current label and a few example rows.
type SampleReport struct {
	Name   string
	Source string // upstream citation, e.g. "EQMacEmu/Server common/spdat.h"
	Codes  []SampleReportCode
}

// SampleReportCode is one (code, label, examples) tuple.
type SampleReportCode struct {
	Code    int
	Label   string      // "" if the code has no label (unknown)
	Samples []SampleRow // empty if the AuditDef has no Sample function
	Count   int         // distinct rows in the DB carrying this code; 0 if unknown
}

// labelLookup is a tiny indirection so BuildSampleReport can resolve a
// code → label without each enum file having to expose its private map.
// Indexed by AuditDef.Name.
var labelLookup = map[string]func(int) string{}

// registerLabels lets enum files declare their label resolver.
func registerLabels(name string, fn func(int) string) {
	labelLookup[name] = fn
}

// enumSources documents the canonical upstream for each enum, so the
// sample report tells you where to verify if a label looks wrong.
// Indexed by AuditDef.Name. Keep in sync as enums are added.
var enumSources = map[string]string{}

func registerSource(name, src string) { enumSources[name] = src }

// BuildSampleReport runs every AuditDef and produces, for each enum,
// the list of observed codes with their current label and a few example
// rows pulled from the DB. The output is intended to be rendered as
// markdown and visually checked against pqdi / EQMacEmu source.
//
// `limit` caps the number of example rows per code (typical: 3).
func BuildSampleReport(db *sql.DB, limit int) ([]SampleReport, error) {
	defs := Defs()
	out := make([]SampleReport, 0, len(defs))
	for _, def := range defs {
		observed, err := def.Extract(db)
		if err != nil {
			return nil, fmt.Errorf("extract %s: %w", def.Name, err)
		}
		sort.Ints(observed)

		// Tally distinct-code counts via a quick repeat-extract count.
		// Extract already returns distinct codes, so we count rows by
		// re-running Sample when present; otherwise leave Count=0.
		codes := make([]SampleReportCode, 0, len(observed))
		labelFn := labelLookup[def.Name]
		for _, code := range observed {
			rc := SampleReportCode{Code: code}
			if labelFn != nil {
				rc.Label = labelFn(code)
			}
			if def.Sample != nil {
				rows, err := def.Sample(db, code, limit)
				if err != nil {
					return nil, fmt.Errorf("sample %s code %d: %w", def.Name, code, err)
				}
				rc.Samples = rows
			}
			codes = append(codes, rc)
		}
		out = append(out, SampleReport{
			Name:   def.Name,
			Source: enumSources[def.Name],
			Codes:  codes,
		})
	}
	return out, nil
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
