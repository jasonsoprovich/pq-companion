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
type KeyDef struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Components  []Component `json:"components"`
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
			Name:        "Temple of Ssraeshza (Emperor Access)",
			Description: "The Ssraeshzian Insignia grants access to the Temple of Ssraeshza. Defeating the nine high priests allows progression to Emperor Ssraeshza's chamber deep within the temple.",
			Components: []Component{
				{
					ItemID:   19718,
					ItemName: "Ssraeshzian Insignia",
					Notes:    "Obtained by completing the Ssraeshza Temple access quest.",
				},
			},
		},
		{
			ID:          "vex_thal",
			Name:        "Vex Thal",
			Description: "Access to Vex Thal is a server-side progression flag granted after defeating Emperor Ssraeshza in the Temple of Ssraeshza. There is no physical key item — all raid members must be individually flagged by completing the SSRA Emperor encounter.",
			Components:  []Component{},
		},
	}
}
