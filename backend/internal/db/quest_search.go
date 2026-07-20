package db

import (
	"log/slog"
	"sort"
	"strings"
	"sync"
)

// QuestSummary is one browsable quest (identified by its giver NPC + zone) for
// the database explorer's Quests section. Rewards/TurnIns are the resolved item
// refs (for the collapsed card); Dialogue is the full resolved walkthrough (for
// the expanded view), so no second request is needed.
type QuestSummary struct {
	NPC           string              `json:"npc"`
	ZoneShortName string              `json:"zone_short_name"`
	ZoneName      string              `json:"zone_name"`
	Rewards       []ItemRef           `json:"rewards"`
	TurnIns       []ItemRef           `json:"turnins"`
	Dialogue      []QuestDialogueLine `json:"dialogue,omitempty"`
}

// QuestSearchResult is a page of quest summaries plus the unpaged total. The
// `items` field name matches the generic SearchResult<T> wrapper the frontend
// uses for every other explorer resource.
type QuestSearchResult struct {
	Results []QuestSummary `json:"items"`
	Total   int            `json:"total"`
}

type questSearchEntry struct {
	summary  QuestSummary
	haystack string // lowercased NPC + zone + every related item name
}

var (
	questSearchOnce  sync.Once
	questSearchIndex []questSearchEntry
)

// buildQuestSearchIndex resolves every quest entry's item and zone names once
// and precomputes a search haystack, so SearchQuests is a cheap in-memory scan.
func (db *DB) buildQuestSearchIndex() {
	questSearchOnce.Do(func() {
		loadQuestSources()

		// One pass to load every item id→name we reference (cheaper and simpler
		// than chunked IN-clauses; the whole items table is ~27k rows).
		names := map[int]string{}
		if rows, err := db.Query("SELECT id, name FROM items"); err == nil {
			for rows.Next() {
				var id int
				var name string
				if err := rows.Scan(&id, &name); err != nil {
					slog.Warn("quest search index: scan item row", "err", err)
					continue
				}
				names[id] = name
			}
			if err := rows.Err(); err != nil {
				slog.Warn("quest search index: iterate item rows", "err", err)
			}
			rows.Close()
		} else {
			slog.Warn("quest search index: query items", "err", err)
		}
		// Same pass, but for faction_list (~2100 rows) — resolves the
		// dialogue-embedded faction deltas' names.
		factionNames := map[int]string{}
		if rows, err := db.Query("SELECT id, name FROM faction_list"); err == nil {
			for rows.Next() {
				var id int
				var name string
				if err := rows.Scan(&id, &name); err != nil {
					slog.Warn("quest search index: scan faction row", "err", err)
					continue
				}
				factionNames[id] = name
			}
			if err := rows.Err(); err != nil {
				slog.Warn("quest search index: iterate faction rows", "err", err)
			}
			rows.Close()
		} else {
			slog.Warn("quest search index: query factions", "err", err)
		}
		zoneLong := map[string]string{}
		resolveZone := func(short string) string {
			if v, ok := zoneLong[short]; ok {
				return v
			}
			v := short
			if z, err := db.GetZoneByShortName(short); err == nil && z != nil && z.LongName != "" {
				v = z.LongName
			}
			zoneLong[short] = v
			return v
		}
		sortedRefs := func(ids []int) []ItemRef {
			refs := make([]ItemRef, 0, len(ids))
			for _, id := range ids {
				refs = append(refs, ItemRef{ID: id, Name: names[id]})
			}
			sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
			return refs
		}
		ref := refItemsFunc(names) // order-preserving, for dialogue
		refFactions := refFactionsFunc(factionNames)

		questSearchIndex = make([]questSearchEntry, 0, len(questEntries))
		for i := range questEntries {
			e := &questEntries[i]
			zoneName := resolveZone(e.Zone)
			rewards := sortedRefs(e.Rewards)
			turnins := sortedRefs(e.TurnIns)
			var hay strings.Builder
			hay.WriteString(strings.ToLower(e.NPC))
			hay.WriteByte('\n')
			hay.WriteString(strings.ToLower(zoneName))
			hay.WriteByte('\n')
			hay.WriteString(strings.ToLower(e.Zone))
			for _, r := range rewards {
				hay.WriteByte('\n')
				hay.WriteString(strings.ToLower(r.Name))
			}
			for _, r := range turnins {
				hay.WriteByte('\n')
				hay.WriteString(strings.ToLower(r.Name))
			}
			questSearchIndex = append(questSearchIndex, questSearchEntry{
				summary: QuestSummary{
					NPC:           e.NPC,
					ZoneShortName: e.Zone,
					ZoneName:      zoneName,
					Rewards:       rewards,
					TurnIns:       turnins,
					Dialogue:      resolveDialogue(e.Dialogue, ref, refFactions),
				},
				haystack: hay.String(),
			})
		}
		sort.Slice(questSearchIndex, func(i, j int) bool {
			a, b := questSearchIndex[i].summary, questSearchIndex[j].summary
			if a.ZoneName != b.ZoneName {
				return a.ZoneName < b.ZoneName
			}
			return a.NPC < b.NPC
		})
	})
}

// SearchQuests returns quest summaries whose NPC, zone, or any related item name
// contains every whitespace-separated term in query (case-insensitive, AND).
// An empty query returns all quests. Results are paged by limit/offset.
func (db *DB) SearchQuests(query string, limit, offset int) QuestSearchResult {
	db.buildQuestSearchIndex()
	terms := strings.Fields(strings.ToLower(query))

	matched := make([]QuestSummary, 0)
	for i := range questSearchIndex {
		ok := true
		for _, t := range terms {
			if !strings.Contains(questSearchIndex[i].haystack, t) {
				ok = false
				break
			}
		}
		if ok {
			matched = append(matched, questSearchIndex[i].summary)
		}
	}

	total := len(matched)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return QuestSearchResult{Results: matched[offset:end], Total: total}
}
