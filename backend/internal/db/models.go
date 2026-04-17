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

// NPC represents a row from the npc_types table.
type NPC struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	LastName string `json:"last_name"`
	Level    int    `json:"level"`
	Race     int    `json:"race"`
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
}

// Zone represents a row from the zone table.
type Zone struct {
	ID          int     `json:"id"`
	ShortName   string  `json:"short_name"`
	LongName    string  `json:"long_name"`
	FileName    string  `json:"file_name"`
	ZoneIDNumber int    `json:"zone_id_number"`
	SafeX       float64 `json:"safe_x"`
	SafeY       float64 `json:"safe_y"`
	SafeZ       float64 `json:"safe_z"`
	MinLevel    int     `json:"min_level"`
	Note        string  `json:"note"`
}

// SearchResult wraps paginated query results.
type SearchResult[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}
