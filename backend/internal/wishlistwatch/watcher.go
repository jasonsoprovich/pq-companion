// Package wishlistwatch alerts when an item on any character's wishlist
// appears in the active character's log — a raid officer calling a drop for
// bids, a looted research component, a tradeable item someone is selling.
//
// It is not a user-authored trigger: the "pattern" is a live set of item
// names sourced from the wishlist table, which the trigger engine (static
// regex compiled once at Reload) has no precedent for. Instead it copies the
// /who PVP warning's approach (see cmd/server/main.go, SetOnPVPSighting): on
// a match it broadcasts a synthetic trigger.TriggerFired — with a fabricated
// TriggerID and hardcoded Actions built from Preferences.WishlistWatch — so
// the existing overlay/TTS/sound pipeline renders it without any new
// frontend wiring.
package wishlistwatch

import (
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// CharacterInfo is the minimal character identity the watcher needs.
type CharacterInfo struct {
	ID   int
	Name string
}

// WishlistEntry is one character's wishlisted item, as seen by the watcher.
type WishlistEntry struct {
	CharacterID int
	ItemID      int
}

// ItemInfo is the minimal item identity the watcher needs to render an alert.
type ItemInfo struct {
	Name string
}

// matchEntry is one (item, wishlisting character) pair, keyed in the
// watcher's match set by the item's lowercased name.
type matchEntry struct {
	ItemID        int
	ItemName      string
	CharacterName string
}

// broadcaster is the subset of *ws.Hub the watcher needs. An interface so
// tests can substitute a recorder instead of standing up a real Hub with
// live WebSocket clients.
type broadcaster interface {
	Broadcast(ws.Event)
}

// Watcher scans log lines for wishlisted item names and broadcasts a
// synthetic trigger:fired event when one appears.
type Watcher struct {
	hub        broadcaster
	cfg        *config.Manager
	activeChar func() string // returns the active character name, "" if unknown

	listChars    func() ([]CharacterInfo, error)
	listWishlist func(characterID int) ([]WishlistEntry, error)
	lookupItem   func(itemID int) (ItemInfo, bool)

	mu     sync.RWMutex
	re     *regexp.Regexp
	byName map[string][]matchEntry

	fireMu    sync.Mutex
	lastFired map[int]time.Time // itemID -> last fire time, for CooldownSecs
}

// NewWatcher constructs a Watcher. Call Rebuild before routing lines.
func NewWatcher(
	hub *ws.Hub,
	cfg *config.Manager,
	activeChar func() string,
	listChars func() ([]CharacterInfo, error),
	listWishlist func(characterID int) ([]WishlistEntry, error),
	lookupItem func(itemID int) (ItemInfo, bool),
) *Watcher {
	return &Watcher{
		hub:          hub,
		cfg:          cfg,
		activeChar:   activeChar,
		listChars:    listChars,
		listWishlist: listWishlist,
		lookupItem:   lookupItem,
		lastFired:    make(map[int]time.Time),
	}
}

// Rebuild recomputes the item-name match set from every character's current
// wishlist. Call after any wishlist add/remove and once at startup.
func (w *Watcher) Rebuild() {
	chars, err := w.listChars()
	if err != nil {
		slog.Error("wishlistwatch: rebuild failed listing characters", "err", err)
		return
	}

	byName := make(map[string][]matchEntry)
	seenName := make(map[string]bool)
	var names []string
	for _, c := range chars {
		entries, err := w.listWishlist(c.ID)
		if err != nil {
			slog.Error("wishlistwatch: rebuild failed listing wishlist", "character", c.Name, "err", err)
			continue
		}
		for _, e := range entries {
			info, ok := w.lookupItem(e.ItemID)
			if !ok || info.Name == "" {
				continue
			}
			key := strings.ToLower(info.Name)
			byName[key] = append(byName[key], matchEntry{
				ItemID:        e.ItemID,
				ItemName:      info.Name,
				CharacterName: c.Name,
			})
			if !seenName[key] {
				seenName[key] = true
				names = append(names, info.Name)
			}
		}
	}

	re := compileMatcher(names)
	w.mu.Lock()
	w.byName = byName
	w.re = re
	w.mu.Unlock()
}

// compileMatcher builds a single case-insensitive, word-boundary alternation
// of every wishlisted item name. Names are sorted longest-first so, per Go
// regexp's leftmost-first alternation semantics, "Robe of the Lost Circle"
// wins over a shorter "Robe" also on someone's wishlist at the same position
// — mirroring db.MatchItemNameInText's longest-match convention.
//
// \b relies on the name's first/last character being an ASCII word
// character; a name starting or ending on punctuation (rare) won't bound
// correctly. Acceptable here since the matched set is the user's own small,
// curated wishlist rather than the full item table.
func compileMatcher(names []string) *regexp.Regexp {
	if len(names) == 0 {
		return nil
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	alts := make([]string, len(names))
	for i, n := range names {
		alts[i] = regexp.QuoteMeta(n)
	}
	re, err := regexp.Compile(`(?i)\b(?:` + strings.Join(alts, "|") + `)\b`)
	if err != nil {
		slog.Error("wishlistwatch: failed to compile matcher", "err", err)
		return nil
	}
	return re
}

// itemEffectRe matches EQ's "Your <item> <effect phrase>." lines: focus
// effect procs and clicky activations that happen to name the item worn or
// used. These aren't a loot/sale/link event — the item is already owned by
// whoever is playing, not something to alert on — so lines matching this
// shape are excluded before the wishlist match even runs.
var itemEffectRe = regexp.MustCompile(`(?i)^Your .+ (?:begins to glow|shimmers briefly|flickers with a pale light|feeds you with power|sparkles|feels alive with power|pulses with light as your vision sharpens)\.?\s*$`)

// HandleLine scans a raw log line for wishlisted item names and fires
// alerts for any match. Called alongside trigger.Engine.Handle from the same
// dispatch path, so live and replayed lines both drive it.
func (w *Watcher) HandleLine(msg string) {
	prefs := w.cfg.Get().Preferences.WishlistWatch
	if !prefs.Enabled {
		return
	}

	w.mu.RLock()
	re, byName := w.re, w.byName
	w.mu.RUnlock()
	if re == nil {
		return
	}

	if itemEffectRe.MatchString(msg) {
		return
	}

	active := ""
	if w.activeChar != nil {
		active = w.activeChar()
	}

	matched := make(map[string]bool)
	for _, m := range re.FindAllString(msg, -1) {
		key := strings.ToLower(m)
		if matched[key] {
			continue
		}
		matched[key] = true

		// Group eligible entries by ItemID (not just name) so an alert
		// enumerates every character wishlisting this exact item, and two
		// different items that happen to share a display name still fire
		// — and cooldown — independently.
		byItem := make(map[int][]matchEntry)
		var order []int
		for _, e := range byName[key] {
			isOwn := active != "" && strings.EqualFold(e.CharacterName, active)
			if !isOwn && !prefs.IncludeOtherChars {
				continue
			}
			if _, seen := byItem[e.ItemID]; !seen {
				order = append(order, e.ItemID)
			}
			byItem[e.ItemID] = append(byItem[e.ItemID], e)
		}

		for _, itemID := range order {
			if w.onCooldown(itemID, prefs.CooldownSecs) {
				continue
			}
			w.fire(prefs, byItem[itemID])
		}
	}
}

// onCooldown reports whether itemID fired within the last cooldownSecs
// (0 = use the configured default) and, if not, marks it as fired now.
func (w *Watcher) onCooldown(itemID, cooldownSecs int) bool {
	if cooldownSecs <= 0 {
		cooldownSecs = config.DefaultWishlistWatchCooldownSecs
	}
	window := time.Duration(cooldownSecs) * time.Second

	w.fireMu.Lock()
	defer w.fireMu.Unlock()
	if last, ok := w.lastFired[itemID]; ok && time.Since(last) < window {
		return true
	}
	w.lastFired[itemID] = time.Now()
	return false
}

// fire builds the configured alert actions for entries — every wishlisting
// character for a single ItemID — and broadcasts one synthetic
// trigger:fired event, exactly as the /who PVP warning does. When the item
// is on more than one character's wishlist, {character} expands to all of
// them (e.g. "Bard and Cleric") instead of picking just one.
func (w *Watcher) fire(prefs config.WishlistWatchSettings, entries []matchEntry) {
	if len(entries) == 0 {
		return
	}
	e := entries[0]

	names := make([]string, 0, len(entries))
	seenChar := make(map[string]bool)
	for _, entry := range entries {
		key := strings.ToLower(entry.CharacterName)
		if seenChar[key] {
			continue
		}
		seenChar[key] = true
		names = append(names, entry.CharacterName)
	}

	text := expandTemplate(prefs.Template, e.ItemName, joinNames(names))

	var actions []trigger.Action
	if prefs.OverlayEnabled {
		actions = append(actions, trigger.Action{
			Type:         trigger.ActionOverlayText,
			Text:         text,
			DurationSecs: prefs.OverlayDurationSecs,
			Color:        prefs.OverlayColor,
		})
	}
	if prefs.TTSEnabled {
		actions = append(actions, trigger.Action{
			Type:   trigger.ActionTextToSpeech,
			Text:   text,
			Voice:  prefs.TTSVoice,
			Volume: float64(prefs.TTSVolume) / 100,
		})
	}
	if prefs.SoundEnabled && prefs.SoundPath != "" {
		actions = append(actions, trigger.Action{
			Type:      trigger.ActionPlaySound,
			SoundPath: prefs.SoundPath,
			Volume:    float64(prefs.SoundVolume) / 100,
		})
	}
	if len(actions) == 0 {
		return
	}

	w.hub.Broadcast(ws.Event{
		Type: trigger.WSEventTriggerFired,
		Data: trigger.TriggerFired{
			TriggerID:   "system:wishlist:" + strconv.Itoa(e.ItemID),
			TriggerName: "Wishlist: " + e.ItemName,
			MatchedLine: text,
			Actions:     actions,
			FiredAt:     time.Now(),
		},
	})
}

func expandTemplate(tmpl, item, character string) string {
	if tmpl == "" {
		tmpl = config.DefaultWishlistWatchTemplate
	}
	return strings.NewReplacer("{item}", item, "{character}", character).Replace(tmpl)
}

// joinNames formats character names as a natural English list — "A",
// "A and B", or "A, B, and C" — so the default template's "{character}'s
// wishlist" wording still reads sensibly when an item is wishlisted by more
// than one character.
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	default:
		return strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
	}
}
