package db

import (
	"sort"
	"strings"
)

// ItemRef is a lightweight item reference (id + name) used to render the
// related items in a quest entry — the turn-ins required for a reward, or the
// rewards granted for a turn-in.
type ItemRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ItemQuestRef is one quest's involvement with an item, resolved for display:
// the NPC and zone, plus the related items (the rewards a "used in" quest
// grants in exchange for this item).
type ItemQuestRef struct {
	NPC           string    `json:"npc"`
	ZoneShortName string    `json:"zone_short_name"`
	ZoneName      string    `json:"zone_name"`
	RelatedItems  []ItemRef `json:"related_items,omitempty"`
}

// ItemQuestStep is one resolved step in the questline that yields an item:
// turn in Requires to NPC (in zone) → receive Grants. Ordered prerequisite-
// first by GetItemQuests.
type ItemQuestStep struct {
	NPC           string    `json:"npc"`
	ZoneShortName string    `json:"zone_short_name"`
	ZoneName      string    `json:"zone_name"`
	Requires      []ItemRef `json:"requires,omitempty"`
	Grants        []ItemRef `json:"grants,omitempty"`
}

// ItemQuests backs the item page's Quests tab. Chain is the full questline to
// obtain the item (prerequisite-first); UsedIn lists quests that consume the
// item as a turn-in. Data comes from the embedded quest-script facts
// (quest_sources.go), with zone long-names and item names resolved from
// quarm.db.
type ItemQuests struct {
	Chain  []ItemQuestStep `json:"chain"`
	UsedIn []ItemQuestRef  `json:"used_in"`
}

// GetItemQuests resolves an item's questline and turn-in usages for display.
func (db *DB) GetItemQuests(itemID int) (*ItemQuests, error) {
	chain := QuestChainForItem(itemID)
	_, usedIn := QuestsForItem(itemID)
	out := &ItemQuests{Chain: []ItemQuestStep{}, UsedIn: []ItemQuestRef{}}
	if len(chain) == 0 && len(usedIn) == 0 {
		return out, nil
	}

	// Collect every referenced item id across the chain + turn-in usages for
	// one batch name lookup.
	idSet := map[int]bool{}
	for _, s := range chain {
		for _, id := range s.Requires {
			idSet[id] = true
		}
		for _, id := range s.Grants {
			idSet[id] = true
		}
	}
	for _, q := range usedIn {
		for _, id := range q.Rewards {
			idSet[id] = true
		}
	}
	names, err := db.itemNames(idSet)
	if err != nil {
		return nil, err
	}
	zoneNames := map[string]string{} // short -> long, cached across quests

	refItems := func(ids []int, excludeSelf bool) []ItemRef {
		refs := make([]ItemRef, 0, len(ids))
		for _, id := range ids {
			if excludeSelf && id == itemID {
				continue
			}
			refs = append(refs, ItemRef{ID: id, Name: names[id]})
		}
		sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
		return refs
	}
	resolveZone := func(short string) string {
		if long, ok := zoneNames[short]; ok {
			return long
		}
		long := short
		if z, err := db.GetZoneByShortName(short); err == nil && z != nil && z.LongName != "" {
			long = z.LongName
		}
		zoneNames[short] = long
		return long
	}

	for _, s := range chain {
		out.Chain = append(out.Chain, ItemQuestStep{
			NPC:           s.NPC,
			ZoneShortName: s.Zone,
			ZoneName:      resolveZone(s.Zone),
			Requires:      refItems(s.Requires, false),
			Grants:        refItems(s.Grants, false),
		})
	}
	for _, q := range usedIn {
		out.UsedIn = append(out.UsedIn, ItemQuestRef{
			NPC:           q.NPC,
			ZoneShortName: q.Zone,
			ZoneName:      resolveZone(q.Zone),
			RelatedItems:  refItems(q.Rewards, true),
		})
	}
	sortQuestRefs(out.UsedIn)
	return out, nil
}

func sortQuestRefs(refs []ItemQuestRef) {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].ZoneName != refs[j].ZoneName {
			return refs[i].ZoneName < refs[j].ZoneName
		}
		return refs[i].NPC < refs[j].NPC
	})
}

// itemNames batch-resolves item ids to names in a single query.
func (db *DB) itemNames(idSet map[int]bool) (map[int]string, error) {
	names := map[int]string{}
	if len(idSet) == 0 {
		return names, nil
	}
	ids := make([]string, 0, len(idSet))
	args := make([]any, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, "?")
		args = append(args, id)
	}
	rows, err := db.Query("SELECT id, name FROM items WHERE id IN ("+strings.Join(ids, ",")+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		names[id] = name
	}
	return names, rows.Err()
}
