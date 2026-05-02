package db

import (
	_ "embed"
	"encoding/json"
	"log/slog"
	"strconv"
	"sync"
)

//go:embed aa_descriptions.json
var aaDescriptionsJSON []byte

var (
	aaDescriptionsOnce sync.Once
	aaDescriptions     map[int]string
)

// loadAADescriptions parses the embedded aa_descriptions.json on first call.
// The JSON is keyed by stringified eqmacid → description text, generated
// from a TAKP eqstr_en.txt by cmd/aa-descriptions. Lives outside the SQLite
// dump so it survives every quarm.db regeneration from upstream MySQL.
func loadAADescriptions() map[int]string {
	aaDescriptionsOnce.Do(func() {
		raw := make(map[string]string)
		if err := json.Unmarshal(aaDescriptionsJSON, &raw); err != nil {
			slog.Error("parse aa_descriptions.json", "err", err)
			aaDescriptions = map[int]string{}
			return
		}
		aaDescriptions = make(map[int]string, len(raw))
		for k, v := range raw {
			id, err := strconv.Atoi(k)
			if err != nil {
				continue
			}
			aaDescriptions[id] = v
		}
	})
	return aaDescriptions
}
