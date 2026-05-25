// Package keys defines static key/access-item definitions for Project Quarm zones.
// These are well-known game data and do not require a database lookup.
package keys

// Component is one required item for a key quest or zone access check.
// AltItemIDs lists additional item IDs that also satisfy this component — used
// when a quest accepts "any one of several items" (e.g. Sleeper's Tomb
// talismans). The canonical ItemID is what's shown in the UI.
type Component struct {
	ItemID     int    `json:"item_id"`
	ItemName   string `json:"item_name"` // display only — canonical identifier is ItemID
	Notes      string `json:"notes,omitempty"`
	AltItemIDs []int  `json:"alt_item_ids,omitempty"`
}

// KeyDef describes a zone key or access-item quest.
// FinalItem, when set, is the assembled key item — holding it short-circuits
// the per-component checklist and marks the character as fully keyed.
// IntermediateItem, when set, is an intermediate combine result; holding it
// marks the first IntermediateCoverCount components as complete, while the
// remaining components are still checked individually.
type KeyDef struct {
	ID                    string      `json:"id"`
	Name                  string      `json:"name"`
	Description           string      `json:"description"`
	Components            []Component `json:"components"`
	FinalItem             *Component  `json:"final_item,omitempty"`
	IntermediateItem      *Component  `json:"intermediate_item,omitempty"`
	IntermediateCoverCount int        `json:"intermediate_cover_count,omitempty"`
}

// All returns all key definitions, ordered from classic Kunark through Luclin.
func All() []KeyDef {
	return []KeyDef{
		{
			ID:          "veeshan_peak",
			Name:        "Veeshan's Peak",
			Description: "Requires completing the Ring of Scale key quest with Garudon in Field of Bone. Both components must be in your inventory when you hand in the Tome of Order and Discord.",
			Components: []Component{
				{
					ItemID:   1729,
					ItemName: "Charasis Tome",
					Notes:    "Drops from Hierophant Prime Grekal in Howling Stones (Charasis).",
				},
				{
					ItemID:   18302,
					ItemName: "Book of Scale",
					Notes:    "Drops from Lady Vox (Permafrost) and Lord Nagafen (Sol B).",
				},
			},
		},
		{
			ID:          "sleepers_tomb",
			Name:        "Sleeper's Tomb — Sleeper's Key",
			Description: "Turn in any ONE of the listed Velious talismans to Jaled Dar's shade in Dragon Necropolis to receive the Sleeper's Key.",
			Components: []Component{
				{
					// Single "any one of" component — the canonical display item is
					// Klandicar's (common Western Wastes drop); AltItemIDs accept any
					// of the other accepted talismans.
					ItemID:   27255,
					ItemName: "Sleeper's Tomb Talisman (any one)",
					Notes:    "Klandicar's (Western Wastes), Lendiniara's (Temple of Veeshan), Sontalak's (Western Wastes), Yelinak's (Skyshrine), Zlandicar's (Dragon Necropolis), or Shard of Hsagra's Talisman (Kael / Velketor's).",
					AltItemIDs: []int{
						27259, // Lendiniara's Talisman
						27256, // Sontalak's Talisman
						27266, // Yelinak's Talisman
						27258, // Zlandicar's Talisman
						9296,  // Shard of Hsagra's Talisman
					},
				},
			},
			FinalItem: &Component{
				ItemID:   27265,
				ItemName: "Sleeper's Key",
				Notes:    "Turn the talisman in to Jaled Dar's shade in Dragon Necropolis.",
			},
		},
		{
			ID:          "old_sebilis",
			Name:        "Old Sebilis — Trakanon Idol",
			Description: "Trakanon Idol grants access to Old Sebilis. Both medallions drop from common froglok mobs in Trakanon's Teeth; turn both in to Emperor Ganak (cave in the SW corner of the zone) to receive the idol.",
			Components: []Component{
				{
					ItemID:   19951,
					ItemName: "Medallion of the Kunzar",
					Notes:    "Drops from a froglok forager (common spawn around the central lake in Trakanon's Teeth).",
				},
				{
					ItemID:   19952,
					ItemName: "Medallion of the Nathsar",
					Notes:    "Drops from a froglok hunter (common spawn around the central lake in Trakanon's Teeth).",
				},
			},
			FinalItem: &Component{
				ItemID:   20883,
				ItemName: "Trakanon Idol",
				Notes:    "Turn in both medallions to Emperor Ganak in Trakanon's Teeth.",
			},
		},
		{
			ID:          "howling_stones",
			Name:        "Howling Stones (Charasis)",
			Description: "The Key to Charasis is required to enter Howling Stones. Obtained via a quest in Kaesora.",
			Components: []Component{
				{
					ItemID:   20600,
					ItemName: "Key to Charasis",
					Notes:    "Quest reward from Zebuxoruk's Cage quest started in Kaesora.",
				},
			},
		},
		{
			ID:          "katta_castellum",
			Name:        "Katta Castellum",
			Description: "A Katta Castellum Badge of Service is required to enter the city of Katta Castellum on Luclin.",
			Components: []Component{
				{
					ItemID:   31752,
					ItemName: "Katta Castellum Badge of Service",
					Notes:    "Obtained via citizenship quest with the Combine ambassador.",
				},
			},
		},
		{
			ID:          "arx_seru",
			Name:        "Arx Seru",
			Description: "The Arx Key is required to access Arx Seru on Luclin. Obtained from Arbitor Xxylm in Katta Castellum after completing the citizenship quest chain.",
			Components: []Component{
				{
					ItemID:   3650,
					ItemName: "Arx Key",
					Notes:    "Obtained from Arbitor Xxylm in Katta Castellum.",
				},
			},
		},
		{
			ID:          "ssra_emperor",
			Name:        "Temple of Ssraeshza — Ring of the Shissar (Emperor Access)",
			Description: "The Ring of the Shissar is required to access Emperor Ssraeshza's chamber in the Temple of Ssraeshza. It is assembled in a Taskmaster's Pouch from three components dropped by named mobs throughout the temple.",
			Components: []Component{
				{
					ItemID:   17118,
					ItemName: "Taskmaster's Pouch",
					Notes:    "Combine container — drops in Ssraeshza Temple.",
				},
				{
					ItemID:   19716,
					ItemName: "Zazuzh's Idol",
					Notes:    "Drops from Vyzh`dra the Cursed in Ssraeshza Temple.",
				},
				{
					ItemID:   19717,
					ItemName: "Zeruzsh's Ring",
					Notes:    "Drops from Vyzh`dra the Exiled in Ssraeshza Temple.",
				},
				{
					ItemID:   19718,
					ItemName: "Ssraeshzian Insignia",
					Notes:    "Drops from Diabo Xi Va Xakra in Ssraeshza Temple.",
				},
			},
			FinalItem: &Component{
				ItemID:   19719,
				ItemName: "Ring of the Shissar",
				Notes:    "Assembled key — holding it grants Emperor Ssraeshza access.",
			},
		},
		{
			ID:          "vex_thal",
			Name:        "Vex Thal — The Scepter of Shadows",
			Description: "The Scepter of Shadows is the key to Vex Thal. Combine 10 Lucid Shards (one from each Luclin zone) in a Shadowed Scepter Frame to make the Unadorned Scepter of Shadows, then combine A Planes Rift and A Glowing Orb of Luclinite inside the Unadorned Scepter to forge The Scepter of Shadows.",
			Components: []Component{
				{
					ItemID:   17323,
					ItemName: "Shadowed Scepter Frame",
					Notes:    "Combine container for the 10 Lucid Shards. Quest reward in Sanctus Seru.",
				},
				{ItemID: 22185, ItemName: "A Lucid Shard (The Grey)", Notes: "Drops in The Grey."},
				{ItemID: 22186, ItemName: "A Lucid Shard (Fungus Grove)", Notes: "Drops in The Fungus Grove."},
				{ItemID: 22187, ItemName: "A Lucid Shard (Scarlet Desert)", Notes: "Drops in the Scarlet Desert."},
				{ItemID: 22188, ItemName: "A Lucid Shard (The Deep)", Notes: "Drops in The Deep."},
				{ItemID: 22189, ItemName: "A Lucid Shard (Ssraeshza Temple)", Notes: "Drops in Ssraeshza Temple."},
				{ItemID: 22190, ItemName: "A Lucid Shard (Akheva Ruins)", Notes: "Drops in Akheva Ruins."},
				{ItemID: 22191, ItemName: "A Lucid Shard (Dawnshroud Peaks)", Notes: "Drops in The Dawnshroud Peaks."},
				{ItemID: 22192, ItemName: "A Lucid Shard (Maiden's Eye)", Notes: "Drops in The Maiden's Eye."},
				{ItemID: 22193, ItemName: "A Lucid Shard (Acrylia Caverns)", Notes: "Drops in Acrylia Caverns."},
				{ItemID: 22194, ItemName: "A Lucid Shard (Katta Castellum / Sanctus Seru)", Notes: "Drops in Katta Castellum and Sanctus Seru."},
				{
					ItemID:   9410,
					ItemName: "A Planes Rift",
					Notes:    "Final-combine ingredient — drops from planar bosses.",
				},
				{
					ItemID:   22196,
					ItemName: "A Glowing Orb of Luclinite",
					Notes:    "Final-combine ingredient — drops from Akheva Ruins / Vex Thal area.",
				},
			},
			// IntermediateItem: holding the Unadorned Scepter of Shadows means the
			// Frame + 10 Lucid Shards combine has already been done (those items were
			// consumed). The first 11 components (Frame + shards) are marked covered.
			IntermediateItem: &Component{
				ItemID:   17324,
				ItemName: "Unadorned Scepter of Shadows",
				Notes:    "Result of combining the Frame + 10 Lucid Shards. Still needs A Planes Rift and A Glowing Orb of Luclinite.",
			},
			IntermediateCoverCount: 11,
			FinalItem: &Component{
				ItemID:   22198,
				ItemName: "The Scepter of Shadows",
				Notes:    "Assembled key — holding it grants Vex Thal access.",
			},
		},
	}
}
