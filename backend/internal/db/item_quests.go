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
// the NPC and zone, plus the related items (turn-ins for a "rewarded by" quest,
// rewards for a "used in" quest).
type ItemQuestRef struct {
	NPC           string    `json:"npc"`
	ZoneShortName string    `json:"zone_short_name"`
	ZoneName      string    `json:"zone_name"`
	RelatedItems  []ItemRef `json:"related_items,omitempty"`
}

// ItemQuests holds the quests that reward an item and the quests that consume
// it as a turn-in. Backs the item page's Quests tab. Data comes from the
// embedded quest-script facts (quest_sources.go), with zone long-names and
// related item names resolved from quarm.db.
type ItemQuests struct {
	RewardedBy []ItemQuestRef `json:"rewarded_by"`
	UsedIn     []ItemQuestRef `json:"used_in"`
}

// GetItemQuests resolves the quest sources for an item into a display-ready
// shape. "Rewarded by" lists quests that grant the item (with the turn-ins
// they require); "used in" lists quests that consume the item as a turn-in
// (with the rewards they grant). The item itself is excluded from the related
// lists so it isn't shown as its own prerequisite/reward.
func (db *DB) GetItemQuests(itemID int) (*ItemQuests, error) {
	rewardedBy, usedIn := QuestsForItem(itemID)
	out := &ItemQuests{RewardedBy: []ItemQuestRef{}, UsedIn: []ItemQuestRef{}}
	if len(rewardedBy) == 0 && len(usedIn) == 0 {
		return out, nil
	}

	// Collect every related item id across all quests for one batch name lookup.
	idSet := map[int]bool{}
	for _, q := range rewardedBy {
		for _, id := range q.TurnIns {
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

	refItems := func(ids []int) []ItemRef {
		refs := make([]ItemRef, 0, len(ids))
		for _, id := range ids {
			if id == itemID {
				continue // don't list the item as its own prerequisite/reward
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

	for _, q := range rewardedBy {
		out.RewardedBy = append(out.RewardedBy, ItemQuestRef{
			NPC:           q.NPC,
			ZoneShortName: q.Zone,
			ZoneName:      resolveZone(q.Zone),
			RelatedItems:  refItems(q.TurnIns),
		})
	}
	for _, q := range usedIn {
		out.UsedIn = append(out.UsedIn, ItemQuestRef{
			NPC:           q.NPC,
			ZoneShortName: q.Zone,
			ZoneName:      resolveZone(q.Zone),
			RelatedItems:  refItems(q.Rewards),
		})
	}
	sortQuestRefs(out.RewardedBy)
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
