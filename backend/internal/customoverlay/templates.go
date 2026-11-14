package customoverlay

// Template is a ready-made overlay definition offered by "+ New custom
// overlay". The def has no ID; creating from it is an ordinary Create.
type Template struct {
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Def         OverlayDef `json:"def"`
}

// Templates returns the built-in templates. Each call returns fresh copies,
// so callers may edit them.
func Templates() []Template {
	return []Template{dkpBidTemplate()}
}

// dkpBidTemplate reads a loot master's guild-chat calls ("Bids open on
// <item>", "<item> CLOSING", "SOLD! <item> to <name> for <amount>") and
// bidders' bare-number guild bids. Guilds are expected to adjust the wording
// to match their own loot master's macros.
func dkpBidTemplate() Template {
	return Template{
		Key:  "dkp-bids",
		Name: "DKP bid tracker",
		Description: "Tracks an auction run in guild chat: the item up for bid, " +
			"each bidder's highest bid, a countdown, and the winner. Edit the " +
			"rules to match your loot master's wording.",
		Def: OverlayDef{
			Name:    "DKP Bids",
			Enabled: true,
			Widgets: []Widget{
				{ID: "status", Type: WidgetFields, Fields: []Field{
					{Key: "item", Label: "Item"},
					{Key: "status", Label: "Status", Colors: []ColorRule{
						{Contains: "open", Color: "#4ade80"},
						{Contains: "closing", Color: "#fbbf24"},
						{Contains: "sold", Color: "#60a5fa"},
					}},
					{Key: "winner", Label: "Winner"},
				}},
				{ID: "bids", Type: WidgetTable, Title: "Bids",
					Columns: []Column{{Key: "bidder", Label: "Bidder"}, {Key: "amount", Label: "DKP"}},
					SortBy:  "amount", SortDesc: true,
					Merge: MergeHighest, MergeColumn: "amount"},
				{ID: "timer", Type: WidgetCountdown, Title: "Closes in", Seconds: 60},
				{ID: "log", Type: WidgetFeed, Title: "Recent", MaxLines: 5},
			},
			Rules: []Rule{
				{ID: "open", Name: "Bids open", Enabled: true, Channel: ChannelGuild,
					Simple: &SimpleMatch{Kind: SimpleStarts, Text: "Bids open on {item}"},
					Ops: []Op{
						{Type: OpResetWidget, Widget: "bids"},
						{Type: OpResetWidget, Widget: "log"},
						{Type: OpSetField, Widget: "status", Field: "item", Value: "{item}"},
						{Type: OpSetField, Widget: "status", Field: "status", Value: "Open"},
						{Type: OpClearField, Widget: "status", Field: "winner"},
						{Type: OpCountdownStart, Widget: "timer"},
					}},
				{ID: "bid", Name: "Bid", Enabled: true, Channel: ChannelGuild, Direction: DirectionIn,
					Simple: &SimpleMatch{Kind: SimpleExact, Text: "{amount:number}"},
					Ops: []Op{
						{Type: OpUpsertRow, Widget: "bids", Key: "{speaker}", Cells: map[string]string{"amount": "{amount}"}},
						{Type: OpAddLine, Widget: "log", Value: "{speaker} bid {amount}"},
					}},
				{ID: "closing", Name: "Closing", Enabled: true, Channel: ChannelGuild,
					Simple: &SimpleMatch{Kind: SimpleContains, Text: "CLOSING"},
					Ops:    []Op{{Type: OpSetField, Widget: "status", Field: "status", Value: "CLOSING"}}},
				{ID: "sold", Name: "Sold", Enabled: true, Channel: ChannelGuild,
					Simple: &SimpleMatch{Kind: SimpleStarts, Text: "SOLD! {item} to {winner} for {amount:number}"},
					Ops: []Op{
						{Type: OpSetField, Widget: "status", Field: "status", Value: "SOLD"},
						{Type: OpSetField, Widget: "status", Field: "winner", Value: "{winner} ({amount})"},
						{Type: OpCountdownStop, Widget: "timer"},
					}},
			},
		},
	}
}
