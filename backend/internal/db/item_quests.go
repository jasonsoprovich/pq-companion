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

// QuestDialogueLine is one resolved branch of a quest walkthrough: the player
// action that triggers it (Triggers = say keywords, TurnIn = items handed in;
// both empty = hail/unconditional), the NPC's spoken Text, and any Grants.
type QuestDialogueLine struct {
	Triggers []string  `json:"triggers,omitempty"`
	TurnIn   []ItemRef `json:"turnin,omitempty"`
	Text     string    `json:"text,omitempty"`
	Grants   []ItemRef `json:"grants,omitempty"`
}

// WalkthroughQuest is one NPC's full quest walkthrough, resolved for display.
type WalkthroughQuest struct {
	NPC           string              `json:"npc"`
	ZoneShortName string              `json:"zone_short_name"`
	ZoneName      string              `json:"zone_name"`
	Dialogue      []QuestDialogueLine `json:"dialogue"`
}

// ItemQuests backs the item page's Quests tab. Walkthrough is the full,
// self-contained dialogue of the NPC(s) that reward the item (say a keyword /
// turn in items → NPC response + items); UsedIn lists quests that consume the
// item as a turn-in. Data comes from the embedded quest-script walkthrough
// (quest_sources.go), with zone long-names and item names resolved from
// quarm.db.
type ItemQuests struct {
	Walkthrough []WalkthroughQuest `json:"walkthrough"`
	UsedIn      []ItemQuestRef     `json:"used_in"`
}

// GetItemQuests resolves an item's quest walkthroughs and turn-in usages.
func (db *DB) GetItemQuests(itemID int) (*ItemQuests, error) {
	rewardedBy, usedIn := QuestsForItem(itemID)
	out := &ItemQuests{Walkthrough: []WalkthroughQuest{}, UsedIn: []ItemQuestRef{}}
	if len(rewardedBy) == 0 && len(usedIn) == 0 {
		return out, nil
	}

	// Collect every referenced item id for one batch name lookup.
	idSet := map[int]bool{}
	for _, q := range rewardedBy {
		for _, b := range q.Dialogue {
			for _, id := range b.TurnIn {
				idSet[id] = true
			}
			for _, id := range b.Grants {
				idSet[id] = true
			}
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
	ref := refItemsFunc(names)
	resolveZone := db.zoneResolver()

	for _, q := range rewardedBy {
		out.Walkthrough = append(out.Walkthrough, WalkthroughQuest{
			NPC:           q.NPC,
			ZoneShortName: q.Zone,
			ZoneName:      resolveZone(q.Zone),
			Dialogue:      resolveDialogue(q.Dialogue, ref),
		})
	}
	for _, q := range usedIn {
		refs := ref(q.Rewards)
		// Don't list the item as its own reward.
		filtered := refs[:0]
		for _, r := range refs {
			if r.ID != itemID {
				filtered = append(filtered, r)
			}
		}
		out.UsedIn = append(out.UsedIn, ItemQuestRef{
			NPC:           q.NPC,
			ZoneShortName: q.Zone,
			ZoneName:      resolveZone(q.Zone),
			RelatedItems:  filtered,
		})
	}
	sortQuestRefs(out.UsedIn)
	return out, nil
}

// resolveDialogue turns raw quest dialogue branches into display-ready lines,
// resolving the turn-in and granted item ids to refs via ref.
func resolveDialogue(lines []QuestDialogue, ref func([]int) []ItemRef) []QuestDialogueLine {
	out := make([]QuestDialogueLine, 0, len(lines))
	for _, b := range lines {
		out = append(out, QuestDialogueLine{
			Triggers: b.Triggers,
			TurnIn:   ref(b.TurnIn),
			Text:     b.Text,
			Grants:   ref(b.Grants),
		})
	}
	return out
}

// refItemsFunc returns a resolver mapping item ids to refs (preserving order).
func refItemsFunc(names map[int]string) func([]int) []ItemRef {
	return func(ids []int) []ItemRef {
		if len(ids) == 0 {
			return nil
		}
		refs := make([]ItemRef, 0, len(ids))
		for _, id := range ids {
			refs = append(refs, ItemRef{ID: id, Name: names[id]})
		}
		return refs
	}
}

// zoneResolver returns a short→long zone-name resolver that caches lookups.
func (db *DB) zoneResolver() func(string) string {
	cache := map[string]string{}
	return func(short string) string {
		if long, ok := cache[short]; ok {
			return long
		}
		long := short
		if z, err := db.GetZoneByShortName(short); err == nil && z != nil && z.LongName != "" {
			long = z.LongName
		}
		cache[short] = long
		return long
	}
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
