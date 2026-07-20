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
			Name:        "Veeshan's Peak — Key of Veeshan",
			Description: "Collect 9 Medallion Pieces (3 per medallion) and Trakanon's Tooth. Turn each set of 3 pieces in to its respective NPC for an assembled Medallion (Jarsath / Obulus / Kylong), then hand the 3 medallions plus Trakanon's Tooth to Emperor Ganak in Trakanon's Teeth for the Key of Veeshan. Holding an assembled medallion covers its 3 piece rows; holding the Key of Veeshan covers everything.",
			Components: []Component{
				// ── Medallion of the Jarsath (turn-in: Xiblin Fizzlebik, Timorous Deep) ──
				{
					ItemID:     19961,
					ItemName:   "Piece of a Medallion (Jarsath — Top)",
					Notes:      "Ground spawn in Swamp of No Hope (shoreline near +40, +2900).",
					AltItemIDs: []int{19954}, // covered by holding Medallion of the Jarsath
				},
				{
					ItemID:     19960,
					ItemName:   "Piece of a Medallion (Jarsath — Middle)",
					Notes:      "Drops from an ancient Jarsath (roaming undead, east side of Firiona Vie).",
					AltItemIDs: []int{19954},
				},
				{
					ItemID:     19959,
					ItemName:   "Piece of a Medallion (Jarsath — Bottom)",
					Notes:      "Drops from a bloodgill marauder underwater in front of Veksar (Lake of Ill Omen).",
					AltItemIDs: []int{19954},
				},
				// ── Medallion of the Obulus (turn-in: Slixin Klex, Burning Woods) ──
				{
					ItemID:     19958,
					ItemName:   "Piece of a Medallion (Obulus — Top)",
					Notes:      "Hand a Burnished Wooden Stave (Chardok drop) to Ssolet Dnaas on a small island in Warsliks Wood.",
					AltItemIDs: []int{19953}, // covered by holding Medallion of the Obulus
				},
				{
					ItemID:     19957,
					ItemName:   "Piece of a Medallion (Obulus — Middle)",
					Notes:      "Drops from rotting skeleton (PH plaguebone skeleton at +2250, -5150) in Dreadlands.",
					AltItemIDs: []int{19953},
				},
				{
					ItemID:     19956,
					ItemName:   "Piece of a Medallion (Obulus — Bottom)",
					Notes:      "Drops from pained soul (PH spectral keeper near Sebilis) in Trakanon's Teeth.",
					AltItemIDs: []int{19953},
				},
				// ── Medallion of the Kylong (turn-in: Professor Akabao, Lake of Ill Omen) ──
				{
					ItemID:     19964,
					ItemName:   "Piece of a Medallion (Kylong — Top)",
					Notes:      "Hand a Black Sapphire + Ruby to Niblek in the Chardok mines, or rare drop from a Di`zok royal guard / Di`zok Guardian.",
					AltItemIDs: []int{19955}, // covered by holding Medallion of the Kylong
				},
				{
					ItemID:     19963,
					ItemName:   "Piece of a Medallion (Kylong — Middle)",
					Notes:      "Drops from Verix Kylox's Remains (rare PH decayed kylong iksar) in Karnor's basement.",
					AltItemIDs: []int{19955},
				},
				{
					ItemID:     19962,
					ItemName:   "Piece of a Medallion (Kylong — Bottom)",
					Notes:      "Ground spawn on the second floor of the library in Kaesora.",
					AltItemIDs: []int{19955},
				},
				// ── Final handin ingredient ──────────────────────────────────────────
				{
					ItemID:   7276,
					ItemName: "Trakanon's Tooth",
					Notes:    "Loot from Trakanon in Old Sebilis.",
				},
			},
			FinalItem: &Component{
				ItemID:   20884,
				ItemName: "Key of Veeshan",
				Notes:    "Turn the 3 assembled Medallions + Trakanon's Tooth in to Emperor Ganak in Trakanon's Teeth.",
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
			Name:        "Howling Stones (Charasis) — Key to Charasis",
			Description: "Loot a Jade Chokidai Prod (from the Chancellor of Di`zok in Lake of Ill Omen) and a Xalgozian Fang (from Xalgoz in Kaesora), then turn both in to Dugroz in Swamp of No Hope to receive the Key to Charasis.",
			Components: []Component{
				{
					ItemID:     7254,
					ItemName:   "Jade Chokidai Prod",
					Notes:      "Drops from Chancellor of Di`zok (roams the western half of Lake of Ill Omen, often the sarnak fort). Tradable — check the Bazaar.",
					AltItemIDs: []int{7256}, // duplicate item id in Quarm db (same name)
				},
				{
					ItemID:   7236,
					ItemName: "Xalgozian Fang",
					Notes:    "Drops from Xalgoz in Kaesora (has a PH).",
				},
			},
			FinalItem: &Component{
				ItemID:   20600,
				ItemName: "Key to Charasis",
				Notes:    "Hand both items to Dugroz in Swamp of No Hope.",
			},
		},
		{
			ID:          "hand_of_glory",
			Name:        "Hand of Glory (Charasis door key)",
			Description: "The Hand of Glory unlocks the locked interior doors in Howling Stones (Charasis). Loot 4 ground spawns from the zone (one per wing) and combine the 3 talons/ring inside the Blood Encrusted Glove (a 4-slot container) to create the Hand of Glory. All ground spawns except the Glove are tradable.",
			Components: []Component{
				{
					ItemID:   20523,
					ItemName: "Intricate Toothed Ring",
					Notes:    "Ground spawn in Howling Stones (one wing).",
				},
				{
					ItemID:   20521,
					ItemName: "Ring-scored Talon",
					Notes:    "Ground spawn in Howling Stones (one wing).",
				},
				{
					ItemID:   20522,
					ItemName: "Petrified Talonless Claw",
					Notes:    "Ground spawn in Howling Stones (one wing).",
				},
				{
					ItemID:   17751,
					ItemName: "Blood Encrusted Glove",
					Notes:    "Ground spawn in Howling Stones — 4-slot combine container (no-trade).",
				},
			},
			FinalItem: &Component{
				ItemID:   17274,
				ItemName: "Hand of Glory",
				Notes:    "Combine the 3 talons/ring inside the Blood Encrusted Glove. Click a locked door to add it to your key ring.",
			},
		},
		{
			ID:          "arx_seru",
			Name:        "Arx Seru — Arx Key (Lord Seru access)",
			Description: "Kill the four Praesertum mini-raid mobs on the top floor of Sanctus Seru, loot one shard from each, and turn all four in to Elnerick Augustleaf in Katta Castellum to receive the Arx Key (and a Seal of Katta).",
			Components: []Component{
				{
					ItemID:   29881,
					ItemName: "Shard of the Shoulder",
					Notes:    "Drops from Praesertum Bikun (NW inner temple, Sanctus Seru).",
				},
				{
					ItemID:   29883,
					ItemName: "Shard of the Hand",
					Notes:    "Drops from Praesertum Vantorus (SW inner temple, Sanctus Seru).",
				},
				{
					ItemID:   29882,
					ItemName: "Shard of the Eye",
					Notes:    "Drops from Praesertum Rhugol (NE inner temple, Sanctus Seru).",
				},
				{
					ItemID:   29884,
					ItemName: "Shard of the Heart",
					Notes:    "Drops from Praesertum Matpa (SE inner temple, Sanctus Seru — gates, mana-burn him).",
				},
			},
			FinalItem: &Component{
				ItemID:   3650,
				ItemName: "Arx Key",
				Notes:    "Turn the 4 shards in to Elnerick Augustleaf in Katta Castellum.",
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
				{ItemID: 22193, ItemName: "A Lucid Shard (Acrylia Caverns)", Notes: "Drops in Acrylia Caverns. Also drops from a few NPCs in Grimling Forest, which is often the easier farm."},
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
