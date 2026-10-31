package popflag

import (
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// broadcaster is the subset of *ws.Hub the reminder needs, so tests can
// substitute a recorder.
type broadcaster interface {
	Broadcast(ws.Event)
}

// Reminder turns Consumer hail-reminder callbacks into alerts. Like the
// wishlist watcher and the /who PVP warning it broadcasts a synthetic
// trigger.TriggerFired with hardcoded Actions built from
// Preferences.PoPHailReminder, so the existing overlay/TTS/sound pipeline
// renders it without any new frontend wiring.
type Reminder struct {
	hub broadcaster
	cfg *config.Manager
	now func() time.Time

	mu        sync.Mutex
	lastFired map[string]time.Time // lower(character)+"|"+lower(zone)
}

// NewReminder constructs a Reminder. Pass its Fire method to
// Consumer.SetOnHailReminder.
func NewReminder(hub broadcaster, cfg *config.Manager) *Reminder {
	return &Reminder{hub: hub, cfg: cfg, now: time.Now, lastFired: map[string]time.Time{}}
}

// Fire alerts for the hails available to character in zone, unless the
// reminder is disabled, has no enabled action, or already fired for this
// character and zone within the cooldown.
func (r *Reminder) Fire(character, zone string, hails []HailReminder) {
	if len(hails) == 0 || r.cfg == nil {
		return
	}
	prefs := r.cfg.Get().Preferences.PoPHailReminder
	if !prefs.Enabled {
		return
	}
	if r.onCooldown(character, zone, prefs.CooldownSecs) {
		return
	}

	text := expandReminderTemplate(prefs.Template, hailNPCs(hails), zone, character)

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

	r.hub.Broadcast(ws.Event{
		Type: trigger.WSEventTriggerFired,
		Data: trigger.TriggerFired{
			TriggerID:   "system:pophail:" + strings.ToLower(zone),
			TriggerName: "PoP hail reminder: " + zone,
			MatchedLine: text,
			Actions:     actions,
			FiredAt:     r.now(),
		},
	})
}

func (r *Reminder) onCooldown(character, zone string, cooldownSecs int) bool {
	if cooldownSecs <= 0 {
		cooldownSecs = config.DefaultPoPHailReminderCooldownSecs
	}
	key := strings.ToLower(character) + "|" + strings.ToLower(zone)
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if last, ok := r.lastFired[key]; ok && now.Sub(last) < time.Duration(cooldownSecs)*time.Second {
		return true
	}
	r.lastFired[key] = now
	return false
}

// hailNPCs returns the distinct NPC names to hail, in order — one NPC can
// back two available steps (Grand Librarian Maelin).
func hailNPCs(hails []HailReminder) []string {
	seen := map[string]bool{}
	var out []string
	for _, h := range hails {
		name := h.NPC
		if name == "" {
			name = h.Label
		}
		if seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	return out
}

func expandReminderTemplate(tmpl string, npcs []string, zone, character string) string {
	if tmpl == "" {
		tmpl = config.DefaultPoPHailReminderTemplate
	}
	return strings.NewReplacer(
		"{hails}", joinNames(npcs),
		"{zone}", zone,
		"{character}", character,
	).Replace(tmpl)
}

// joinNames renders a human list: "A", "A and B", "A, B and C".
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
