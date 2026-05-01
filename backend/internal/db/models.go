package db

// Item represents a row from the items table.
// Fields cover stats, combat, effects, and metadata useful for the item explorer.
type Item struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Lore      string `json:"lore"`
	IDFile    string `json:"id_file"`
	ItemClass int    `json:"item_class"` // 0=common, 1=container, 2=book
	ItemType  int    `json:"item_type"`

	// Combat
	Damage  int `json:"damage"`
	Delay   int `json:"delay"`
	Range   int `json:"range"`
	AC      int `json:"ac"`
	BaneAmt  int `json:"bane_amt"`
	BaneBody int `json:"bane_body"`
	BaneRace int `json:"bane_race"`

	// Stats
	HP       int `json:"hp"`
	Mana     int `json:"mana"`
	Strength int `json:"str"`
	Stamina  int `json:"sta"`
	Agility  int `json:"agi"`
	Dexterity int `json:"dex"`
	Wisdom   int `json:"wis"`
	Intelligence int `json:"int"`
	Charisma int `json:"cha"`

	// Resists
	MagicResist   int `json:"mr"`
	ColdResist    int `json:"cr"`
	DiseaseResist int `json:"dr"`
	FireResist    int `json:"fr"`
	PoisonResist  int `json:"pr"`

	// Flags
	Magic  int `json:"magic"`
	NoDrop int `json:"nodrop"`
	NoRent int `json:"norent"`
	Lore2  int `json:"lore_flag"` // lore flag (item is unique/lore)

	// Equipment
	Slots   int `json:"slots"`
	Classes int `json:"classes"`
	Races   int `json:"races"`
	Weight  int `json:"weight"`
	Size    int `json:"size"`

	// Levels
	RecLevel int `json:"rec_level"`
	ReqLevel int `json:"req_level"`

	// Effects
	ClickEffect int    `json:"click_effect"`
	ClickName   string `json:"click_name"`
	ProcEffect  int    `json:"proc_effect"`
	ProcName    string `json:"proc_name"`
	WornEffect  int    `json:"worn_effect"`
	WornName    string `json:"worn_name"`
	FocusEffect int    `json:"focus_effect"`
	FocusName   string `json:"focus_name"`

	// Container
	BagSize  int `json:"bag_size"`
	BagSlots int `json:"bag_slots"`
	BagType  int `json:"bag_type"`

	// Stack
	Stackable int `json:"stackable"`
	StackSize int `json:"stack_size"`

	Price    int    `json:"price"`
	Icon     int    `json:"icon"`
	MinStatus int   `json:"min_status"`
}

// ItemFilter holds filter parameters for SearchItems.
// Zero values mean "no filter" for all fields except ItemType, where -1 means "any".
type ItemFilter struct {
	Query    string
	BaneBody int
	Race     int // single race bit (1=Human, 2=Barbarian, …); 0=any
	Class    int // single class bit (1=Warrior, 2=Cleric, …); 0=any
	MinLevel int // 0=no minimum
	MaxLevel int // 0=no maximum
	Slot     int // slot bitmask; 0=any slot
	ItemType int // -1=any, 0=1H Slashing, 10=Armor, …
	MinSTR   int
	MinSTA   int
	MinAGI   int
	MinDEX   int
	MinWIS   int
	MinINT   int
	MinCHA   int
	MinHP    int
	MinMana  int
	MinAC    int
	MinMR    int
	MinCR    int
	MinDR    int
	MinFR    int
	MinPR    int
	Limit    int
	Offset   int
}

// NPC represents a row from the npc_types table.
type NPC struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	LastName string `json:"last_name"`
	Level    int    `json:"level"`
	Race     int    `json:"race"`
	RaceName string `json:"race_name"`
	Class    int    `json:"class"`
	BodyType int    `json:"body_type"`

	HP          int `json:"hp"`
	Mana        int `json:"mana"`
	MinDmg      int `json:"min_dmg"`
	MaxDmg      int `json:"max_dmg"`
	AttackCount int `json:"attack_count"`

	// Resists / stats
	MR  int `json:"mr"`
	CR  int `json:"cr"`
	DR  int `json:"dr"`
	FR  int `json:"fr"`
	PR  int `json:"pr"`
	AC  int `json:"ac"`
	STR int `json:"str"`
	STA int `json:"sta"`
	DEX int `json:"dex"`
	AGI int `json:"agi"`
	INT int `json:"int"`
	WIS int `json:"wis"`
	CHA int `json:"cha"`

	// Behavior
	AggroRadius  int     `json:"aggro_radius"`
	RunSpeed     float64 `json:"run_speed"`
	Size         float64 `json:"size"`
	RaidTarget   int     `json:"raid_target"`
	RareSpawn    int     `json:"rare_spawn"`

	// Links
	LootTableID    int `json:"loottable_id"`
	MerchantID     int `json:"merchant_id"`
	NPCSpellsID    int `json:"npc_spells_id"`
	NPCFactionID   int `json:"npc_faction_id"`

	// Raw special abilities string; parsed separately via ParseSpecialAbilities.
	SpecialAbilities string `json:"special_abilities"`

	SpellScale float64 `json:"spell_scale"`
	HealScale  float64 `json:"heal_scale"`
	ExpPct     int     `json:"exp_pct"`
}

// FactionHit represents a single faction's standing change when an NPC is killed.
type FactionHit struct {
	FactionID   int    `json:"faction_id"`
	FactionName string `json:"faction_name"`
	Value       int    `json:"value"`
}

// NPCFaction holds the resolved faction info for an NPC.
type NPCFaction struct {
	PrimaryFactionID   int          `json:"primary_faction_id"`
	PrimaryFactionName string       `json:"primary_faction_name"`
	Hits               []FactionHit `json:"hits"`
}

// Spell represents a row from the spells_new table.
// Effect slots are stored as parallel slices for convenience.
type Spell struct {
	ID   int    `json:"id"`
	Name string `json:"name"`

	// Text
	YouCast    string `json:"you_cast"`
	OtherCasts string `json:"other_casts"`
	CastOnYou  string `json:"cast_on_you"`
	CastOnOther string `json:"cast_on_other"`
	SpellFades string `json:"spell_fades"`

	// Timing (milliseconds)
	CastTime     int `json:"cast_time"`
	RecoveryTime int `json:"recovery_time"`
	RecastTime   int `json:"recast_time"`

	// Duration
	BuffDuration        int `json:"buff_duration"`
	BuffDurationFormula int `json:"buff_duration_formula"`

	Mana       int `json:"mana"`
	Range      int `json:"range"`
	AoERange   int `json:"aoe_range"`
	TargetType int `json:"target_type"`
	ResistType int `json:"resist_type"`
	Skill      int `json:"skill"`

	// Effect slots (12 slots each)
	EffectIDs        [12]int `json:"effect_ids"`
	EffectBaseValues [12]int `json:"effect_base_values"`
	EffectLimitValues [12]int `json:"effect_limit_values"`
	EffectMaxValues  [12]int `json:"effect_max_values"`

	// Class levels (15 classes; 255 = cannot cast)
	ClassLevels [15]int `json:"class_levels"`

	Icon    int `json:"icon"`
	NewIcon int `json:"new_icon"`

	IsDiscipline int `json:"is_discipline"`
	Suspendable  int `json:"suspendable"`
	NoDispell    int `json:"no_dispell"`
	ZoneType     int `json:"zone_type"`

	// GoodEffect comes from spells_new.goodEffect: 1 = beneficial buff,
	// 0 = detrimental. Used to classify timer category for spells whose
	// target type doesn't disambiguate (e.g. single-target friendly buffs).
	GoodEffect int `json:"good_effect"`
}

// SpellItemRef is a slim item reference used in spell cross-reference queries.
type SpellItemRef struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	EffectType string `json:"effect_type,omitempty"` // "click", "worn", "proc", "focus", or "" for scroll
}

// SpellCrossRefs holds items that reference a spell, grouped by relationship type.
type SpellCrossRefs struct {
	ScrollItems []SpellItemRef `json:"scroll_items"` // items that teach this spell
	EffectItems []SpellItemRef `json:"effect_items"` // items with this spell as click/worn/proc/focus
}

// Zone represents a row from the zone table.
type Zone struct {
	ID           int     `json:"id"`
	ShortName    string  `json:"short_name"`
	LongName     string  `json:"long_name"`
	FileName     string  `json:"file_name"`
	ZoneIDNumber int     `json:"zone_id_number"`
	SafeX        float64 `json:"safe_x"`
	SafeY        float64 `json:"safe_y"`
	SafeZ        float64 `json:"safe_z"`
	MinLevel     int     `json:"min_level"`
	Note         string  `json:"note"`
	Outdoor      int     `json:"outdoor"`
	Hotzone      int     `json:"hotzone"`
	CanLevitate  int     `json:"can_levitate"`
	CanBind      int     `json:"can_bind"`
	ExpMod       float64 `json:"exp_mod"`
	Expansion    int     `json:"expansion"`
	NPCLevelMin  int     `json:"npc_level_min"`
	NPCLevelMax  int     `json:"npc_level_max"`
}

// LootDropItem is one item within a loot drop entry.
type LootDropItem struct {
	ItemID     int     `json:"item_id"`
	ItemName   string  `json:"item_name"`
	Chance     float64 `json:"chance"`
	Multiplier int     `json:"multiplier"`
}

// LootDrop is one loot drop group within a loottable entry.
type LootDrop struct {
	ID          int            `json:"id"`
	Name        string         `json:"name"`
	Multiplier  int            `json:"multiplier"`
	Probability int            `json:"probability"`
	Items       []LootDropItem `json:"items"`
}

// NPCLootTable holds the resolved loot table for an NPC.
type NPCLootTable struct {
	ID    int        `json:"id"`
	Name  string     `json:"name"`
	Drops []LootDrop `json:"drops"`
}

// ItemSourceNPC is a minimal NPC record used in item source listings.
type ItemSourceNPC struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	ZoneName      string  `json:"zone_name"`
	ZoneShortName string  `json:"zone_short_name"`
	DropRate      float64 `json:"drop_rate,omitempty"`
}

// ItemForageZone is a zone where an item can be obtained via the Forage skill.
type ItemForageZone struct {
	ZoneShortName string `json:"zone_short_name"`
	ZoneName      string `json:"zone_name"`
	Chance        int    `json:"chance"`
}

// ItemGroundSpawnZone is a zone where an item spawns as a ground item.
type ItemGroundSpawnZone struct {
	ZoneShortName string `json:"zone_short_name"`
	ZoneName      string `json:"zone_name"`
	Name          string `json:"name"`
	MaxAllowed    int    `json:"max_allowed"`
	RespawnTimer  int    `json:"respawn_timer"`
}

// ItemTradeskillEntry is a tradeskill recipe that involves an item as a product or ingredient.
type ItemTradeskillEntry struct {
	RecipeID   int    `json:"recipe_id"`
	RecipeName string `json:"recipe_name"`
	Tradeskill int    `json:"tradeskill"`
	Trivial    int    `json:"trivial"`
	Role       string `json:"role"`  // "product" or "ingredient"
	Count      int    `json:"count"` // successcount or componentcount
}

// ItemSources holds the ways to obtain a given item.
type ItemSources struct {
	Drops        []ItemSourceNPC       `json:"drops"`
	Merchants    []ItemSourceNPC       `json:"merchants"`
	ForageZones  []ItemForageZone      `json:"forage_zones"`
	GroundSpawns []ItemGroundSpawnZone `json:"ground_spawns"`
	Tradeskills  []ItemTradeskillEntry `json:"tradeskills"`
}

// NPCSpawnPoint represents a single spawn point for an NPC.
type NPCSpawnPoint struct {
	ID              int     `json:"id"`
	Zone            string  `json:"zone"`
	ZoneName        string  `json:"zone_name"`
	X               float64 `json:"x"`
	Y               float64 `json:"y"`
	Z               float64 `json:"z"`
	RespawnTime     int     `json:"respawn_time"`
	FastRespawnTime int     `json:"fast_respawn_time"`
}

// SpawnGroupMember is one NPC entry within a spawn group.
type SpawnGroupMember struct {
	NPCID  int    `json:"npc_id"`
	Name   string `json:"name"`
	Chance int    `json:"chance"`
}

// NPCSpawnGroup represents a spawn group and all of its NPC members.
type NPCSpawnGroup struct {
	ID              int                `json:"id"`
	Name            string             `json:"name"`
	RespawnTime     int                `json:"respawn_time"`
	FastRespawnTime int                `json:"fast_respawn_time"`
	Members         []SpawnGroupMember `json:"members"`
}

// NPCSpawns holds spawn point and spawn group data for an NPC.
type NPCSpawns struct {
	SpawnPoints []NPCSpawnPoint `json:"spawn_points"`
	SpawnGroups []NPCSpawnGroup `json:"spawn_groups"`
}

// ZoneConnection is a zone reachable via a zone line from a source zone.
type ZoneConnection struct {
	ZoneID    int    `json:"zone_id"`
	ShortName string `json:"short_name"`
	LongName  string `json:"long_name"`
	Expansion int    `json:"expansion"`
}

// ZoneGroundSpawn is an item that spawns on the ground in a zone.
type ZoneGroundSpawn struct {
	ID           int    `json:"id"`
	ItemID       int    `json:"item_id"`
	ItemName     string `json:"item_name"`
	Name         string `json:"name"`
	MaxAllowed   int    `json:"max_allowed"`
	RespawnTimer int    `json:"respawn_timer"`
}

// ZoneForageItem is an item obtainable via the Forage skill in a zone.
type ZoneForageItem struct {
	ID       int    `json:"id"`
	ItemID   int    `json:"item_id"`
	ItemName string `json:"item_name"`
	Chance   int    `json:"chance"`
	Level    int    `json:"level"`
}

// ZoneDropItem is an item that can drop from an NPC in a zone.
type ZoneDropItem struct {
	ItemID   int     `json:"item_id"`
	ItemName string  `json:"item_name"`
	NPCID    int     `json:"npc_id"`
	NPCName  string  `json:"npc_name"`
	Chance   float64 `json:"chance"`
}

// SearchResult wraps paginated query results.
type SearchResult[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}
