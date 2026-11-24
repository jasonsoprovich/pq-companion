package spelltimer

import (
	"log/slog"
	"strings"
	"time"
)

// Renaming self buffs from the Zeal buff window.
//
// Several buff lines share their land text word for word: Guard of Druzzil
// and Group Resist Magic both print "You feel protected from magic.", Voice
// of Quellious and Koadic's Endless Intellect both print "Your mind expands
// beyond the bounds of space and time.". When someone else casts one on the
// player, there is no "You begin casting" line to tell them apart, so the
// timer gets whichever name the land-text trigger or the clicky heuristic
// picked, with that spell's duration.
//
// The pipe's buff window names the buff the client actually has. A self-buff
// timer whose name isn't in the window, and that the window has never
// confirmed, is renamed to the one buff in the window that shares its land
// text and no other timer covers. The same group cast landed on the other
// members in the same second, so their timers from that burst (same old name
// or land-group family) are renamed with it and take the player's corrected
// expiry.

// pipeRenameSiblingWindow is how close another member's timer must have
// started to the player's for both to count as the same group cast.
const pipeRenameSiblingWindow = 2 * time.Second

// spellLandInfo is the part of a spell row the rename needs.
type spellLandInfo struct {
	castOnYou string
	icon      int
	// rune is an absorb-damage buff (SPA 55 melee, SPA 78 spell): it ends
	// early by being used up, which is not a strip.
	rune bool
}

const (
	seAbsorbMelee = 55
	seAbsorbMagic = 78
)

// pipeRenameLookup is the DB data renameFromPipeLocked needs, fetched before
// e.mu is taken: the window's buff names as spell ids, and the land text of
// those spells and of the self-buff timers' spells.
type pipeRenameLookup struct {
	pipeIDs map[string]int
	info    map[int]spellLandInfo
}

// pipeRename is one self-buff timer renamed from the window, for
// syncRenamedSiblingsLocked.
type pipeRename struct {
	self     *ActiveTimer
	siblings []*ActiveTimer
}

// pipeRenameLookupFor gathers the lookup for one snapshot. e.mu must not be
// held.
func (e *Engine) pipeRenameLookupFor(active string, buffs []PipeBuff) pipeRenameLookup {
	l := pipeRenameLookup{pipeIDs: map[string]int{}, info: map[int]spellLandInfo{}}
	for _, b := range buffs {
		name := strings.TrimSpace(b.Name)
		if id := e.pipeSpellID(name); id > 0 {
			l.pipeIDs[name] = id
		}
	}
	ids := map[int]bool{}
	for _, id := range l.pipeIDs {
		ids[id] = true
	}
	e.mu.Lock()
	for _, t := range e.timers {
		if isSelfBuff(t, active) && t.SpellID > 0 {
			ids[t.SpellID] = true
		}
	}
	e.mu.Unlock()
	for id := range ids {
		if in, ok := e.spellLandInfo(id); ok {
			l.info[id] = in
		}
	}
	return l
}

// spellLandInfo returns a spell's self land text and icon, cached. e.mu must
// not be held.
func (e *Engine) spellLandInfo(id int) (spellLandInfo, bool) {
	e.mu.Lock()
	in, ok := e.landInfo[id]
	e.mu.Unlock()
	if ok || e.db == nil {
		return in, ok
	}
	sp, err := e.db.GetSpell(id)
	if err != nil || sp == nil {
		return spellLandInfo{}, false
	}
	in = spellLandInfo{castOnYou: strings.TrimSpace(sp.CastOnYou), icon: sp.NewIcon}
	for _, eff := range sp.EffectIDs {
		if eff == seAbsorbMelee || eff == seAbsorbMagic {
			in.rune = true
		}
	}
	e.mu.Lock()
	if e.landInfo == nil {
		e.landInfo = make(map[int]spellLandInfo)
	}
	e.landInfo[id] = in
	e.mu.Unlock()
	return in, true
}

// renameFromPipeLocked renames misnamed self-buff timers (and their group
// siblings) to the buff the window shows. Combined-name timers ("Resist
// Magic", "Illusion") are left alone: the window already confirms and syncs
// them through addCombinedBuffNames. e.mu must be held.
func (e *Engine) renameFromPipeLocked(active string, l pipeRenameLookup) []pipeRename {
	if len(l.pipeIDs) == 0 {
		return nil
	}
	covered := map[string]bool{}
	for _, t := range e.timers {
		if isSelfBuff(t, active) {
			covered[t.SpellName] = true
		}
	}
	var out []pipeRename
	for key, t := range e.timers {
		if !isSelfBuff(t, active) || t.SpellID <= 0 || e.pipeConfirmed[key] {
			continue
		}
		if _, inWindow := l.pipeIDs[t.SpellName]; inWindow {
			continue
		}
		if landGroupIndex(t.SpellName, 0) >= 0 {
			continue
		}
		text := l.info[t.SpellID].castOnYou
		if text == "" {
			continue
		}
		pick, n := "", 0
		for name, id := range l.pipeIDs {
			if !covered[name] && strings.EqualFold(l.info[id].castOnYou, text) {
				pick = name
				n++
			}
		}
		if n != 1 {
			continue
		}
		oldName, oldID := t.SpellName, t.SpellID
		if !e.renameTimerLocked(key, t, pick, l.pipeIDs[pick], l.info[l.pipeIDs[pick]].icon) {
			continue
		}
		covered[pick] = true
		r := pipeRename{self: t}
		for skey, s := range e.timers {
			if s == t || s.Category != CategoryBuff || isSelfBuff(s, active) {
				continue
			}
			if d := s.StartsAt.Sub(t.StartsAt); d > pipeRenameSiblingWindow || d < -pipeRenameSiblingWindow {
				continue
			}
			if s.SpellName != oldName && !sameBuffFamily(s, oldName, oldID) {
				continue
			}
			if e.renameTimerLocked(skey, s, pick, t.SpellID, t.Icon) {
				r.siblings = append(r.siblings, s)
			}
		}
		slog.Debug("timer-debug: renamed buff from the Zeal buff window",
			"from", oldName, "to", pick, "siblings", len(r.siblings))
		out = append(out, r)
	}
	return out
}

// renameTimerLocked re-keys t under name, carrying the per-key pipe and tick
// state with it. False (and nothing changed) when a timer already holds the
// new key. e.mu must be held.
func (e *Engine) renameTimerLocked(key string, t *ActiveTimer, name string, spellID, icon int) bool {
	// Keep everything after the name (target, spawn-id tag, stack suffix).
	prefix := t.SpellName + timerKeySep
	if !strings.HasPrefix(key, prefix) {
		return false
	}
	newKey := name + timerKeySep + key[len(prefix):]
	if newKey == key {
		return false
	}
	if _, clash := e.timers[newKey]; clash {
		return false
	}
	delete(e.timers, key)
	t.ID = newKey
	t.SpellName = name
	t.SpellID = spellID
	if icon > 0 {
		t.Icon = icon
	}
	e.timers[newKey] = t
	moveKey := func(m map[string]time.Time) {
		if v, ok := m[key]; ok {
			delete(m, key)
			m[newKey] = v
		}
	}
	moveKey(e.tickBase)
	moveKey(e.tickSnapped)
	moveKey(e.pipeMissingSince)
	delete(e.pipeConfirmed, key)
	delete(e.pipeStripCandidate, key)
	return true
}

// syncRenamedSiblingsLocked gives each renamed sibling the player's expiry,
// which the window's tick count has just corrected (the same cast landed on
// everyone at once, so it ends at once). e.mu must be held.
func (e *Engine) syncRenamedSiblingsLocked(renames []pipeRename) {
	for _, r := range renames {
		for _, s := range r.siblings {
			s.ExpiresAt = r.self.ExpiresAt
			if d := s.ExpiresAt.Sub(s.StartsAt).Seconds(); d > 0 {
				s.DurationSeconds = d
			}
		}
	}
}
