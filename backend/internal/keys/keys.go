// Package keys defines static key/access-item definitions for Project Quarm zones.
// These are well-known game data and do not require a database lookup.
package keys

// Component is one required item for a key quest or zone access check.
type Component struct {
	ItemID   int    `json:"item_id"`
	ItemName string `json:"item_name"` // display only — canonical identifier is ItemID
	Notes    string `json:"notes,omitempty"`
}

// KeyDef describes a zone key or access-item quest.
// FinalItem, when set, is the assembled key item — holding it short-circuits
// the per-component checklist and marks the character as fully keyed.
type KeyDef struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Components  []Component `json:"components"`
	FinalItem   *Component  `json:"final_item,omitempty"`
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
			Name:        "Sleeper's Tomb",
			Description: "The Sleeper's Key is obtained by completing the Warders quest chain in Velious. Hand in quest items to the relevant NPC to receive the key and gain access to Sleeper's Tomb.",
			Components: []Component{
				{
					ItemID:   27265,
					ItemName: "Sleeper's Key",
					Notes:    "Reward from the Warders keying quest chain in Velious.",
				},
			},
		},
		{
			ID:          "old_sebilis",
			Name:        "Old Sebilis",
			Description: "A Trakanon's Tooth is required to zone into Old Sebilis. The tooth is carried — it is not consumed on entry.",
			Components: []Component{
				{
					ItemID:   7276,
					ItemName: "Trakanon's Tooth",
					Notes:    "Drops from Trakanon in the pre-keyed section of Old Sebilis.",
				},
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
			ID:          "griegs_end",
			Name:        "Grieg's End",
			Description: "Grieg's Key is required to enter Grieg's End on Luclin.",
			Components: []Component{
				{
					ItemID:   27650,
					ItemName: "Grieg's Key",
					Notes:    "Dropped by mobs inside Grieg's End after defeating certain named.",
				},
			},
		},
		{
			ID:          "grimling_forest_shackle",
			Name:        "Grimling Forest Shackle Pens",
			Description: "The Grimling Shackle Key opens the locked pen area in Grimling Forest on Luclin.",
			Components: []Component{
				{
					ItemID:   6554,
					ItemName: "Grimling Shackle Key",
					Notes:    "Dropped by Grimling guards in Grimling Forest.",
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
				{ItemID: 22185, ItemName: "A Lucid Shard (Acrylia Caverns)", Notes: "Drops in Acrylia Caverns."},
				{ItemID: 22186, ItemName: "A Lucid Shard (Dawnshroud Peaks)", Notes: "Drops in Dawnshroud Peaks."},
				{ItemID: 22187, ItemName: "A Lucid Shard (Echo Caverns)", Notes: "Drops in Echo Caverns."},
				{ItemID: 22188, ItemName: "A Lucid Shard (Fungus Grove)", Notes: "Drops in Fungus Grove."},
				{ItemID: 22189, ItemName: "A Lucid Shard (Grieg's End)", Notes: "Drops in Grieg's End."},
				{ItemID: 22190, ItemName: "A Lucid Shard (Grimling Forest)", Notes: "Drops in Grimling Forest."},
				{ItemID: 22191, ItemName: "A Lucid Shard (Hollowshade Moor)", Notes: "Drops in Hollowshade Moor."},
				{ItemID: 22192, ItemName: "A Lucid Shard (Maiden's Eye)", Notes: "Drops in Maiden's Eye."},
				{ItemID: 22193, ItemName: "A Lucid Shard (Marus Seru)", Notes: "Drops in Marus Seru."},
				{ItemID: 22194, ItemName: "A Lucid Shard (The Deep)", Notes: "Drops in The Deep."},
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
			FinalItem: &Component{
				ItemID:   22198,
				ItemName: "The Scepter of Shadows",
				Notes:    "Assembled key — holding it grants Vex Thal access.",
			},
		},
	}
}
