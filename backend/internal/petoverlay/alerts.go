package petoverlay

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
)

// The Pet overlay's three alerts are evaluated here on a 1s tick, in the
// backend, so they fire exactly once however many dashboard/popout windows
// are open. Each is a synthetic trigger:fired broadcast (a fabricated
// TriggerID and hardcoded Actions), the same pattern as the wishlist watcher
// and the /who PVP warning, so overlay text, TTS, sound and the audio output
// device all work unchanged.

// alertTick is how often alerts are evaluated.
const alertTick = time.Second

// lowHPRearmPoints is how far above the threshold the pet must recover
// before the low-HP alert can fire again.
const lowHPRearmPoints = 10

// buffMissingGrace delays the buff watch after a pet appears, so a fresh
// charm isn't nagged about buffs before the player has had time to cast them.
const buffMissingGrace = 20 * time.Second

// healthFreshForWatch is how recent a `/pet health` reply must be to vouch
// for a watched buff that has no timer (e.g. one another player cast).
const healthFreshForWatch = 60 * time.Second

// alertState is the per-pet firing state; reset whenever a new pet starts.
type alertState struct {
	// charmArmed / charmFired hold the charm timer expiry the charm alert
	// was armed (seen above the threshold) and fired for. Keying by expiry
	// means a recast (new expiry) re-arms it, and a charm whose whole
	// duration is under the threshold never fires at cast time.
	charmArmed time.Time
	charmFired time.Time
	hpArmed    bool
	// missing marks watched spells already alerted as missing; a spell
	// re-arms once it is seen on the pet again.
	missing map[string]bool
}

func newAlertState() alertState {
	return alertState{hpArmed: true, missing: map[string]bool{}}
}

// Start evaluates alerts every second until ctx is cancelled.
func (t *Tracker) Start(ctx context.Context) {
	tick := time.NewTicker(alertTick)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			t.Tick()
		}
	}
}

// Tick evaluates the alerts once.
func (t *Tracker) Tick() {
	if t.cfg.FireAlert == nil {
		return
	}
	prefs := t.settings()
	t.mu.Lock()
	var fires []trigger.TriggerFired
	if t.st.Mode == ModePet {
		now := t.now()
		if f, ok := t.charmAlertLocked(prefs.CharmExpiring, now); ok {
			fires = append(fires, f)
		}
		if f, ok := t.lowHPAlertLocked(prefs.LowHP, now); ok {
			fires = append(fires, f)
		}
		if f, ok := t.buffMissingAlertLocked(prefs, now); ok {
			fires = append(fires, f)
		}
	}
	t.mu.Unlock()
	for _, f := range fires {
		t.cfg.FireAlert(f)
	}
}

func (t *Tracker) charmAlertLocked(a config.PetAlertSettings, now time.Time) (trigger.TriggerFired, bool) {
	if !a.Enabled || t.cfg.CharmTimer == nil {
		return trigger.TriggerFired{}, false
	}
	spell, target, exp, ok := t.cfg.CharmTimer()
	if !ok || !t.charmTimerMatchesLocked(target) {
		return trigger.TriggerFired{}, false
	}
	threshold := time.Duration(orDefault(a.Threshold, config.DefaultPetCharmExpiringSecs)) * time.Second
	left := exp.Sub(now)
	switch {
	case left > threshold:
		t.alerts.charmArmed = exp
		return trigger.TriggerFired{}, false
	case left <= 0 || !t.alerts.charmArmed.Equal(exp) || t.alerts.charmFired.Equal(exp):
		return trigger.TriggerFired{}, false
	}
	t.alerts.charmFired = exp
	secs := int((left + time.Second - 1) / time.Second)
	text := strings.NewReplacer(
		"{pet}", t.petLabelLocked(),
		"{spell}", spell,
		"{secs}", strconv.Itoa(secs),
	).Replace(orDefaultText(a.Template, config.DefaultPetCharmExpiringTemplate))
	return buildAlert("system:pet:charm", "Pet: charm expiring", text, a, now)
}

func (t *Tracker) lowHPAlertLocked(a config.PetAlertSettings, now time.Time) (trigger.TriggerFired, bool) {
	if !a.Enabled || t.st.HPPercent == nil {
		return trigger.TriggerFired{}, false
	}
	hp := *t.st.HPPercent
	threshold := orDefault(a.Threshold, config.DefaultPetLowHPPct)
	if hp >= threshold+lowHPRearmPoints {
		t.alerts.hpArmed = true
	}
	if hp > threshold || !t.alerts.hpArmed {
		return trigger.TriggerFired{}, false
	}
	t.alerts.hpArmed = false
	text := strings.NewReplacer(
		"{pet}", t.petLabelLocked(),
		"{hp}", strconv.Itoa(hp),
	).Replace(orDefaultText(a.Template, config.DefaultPetLowHPTemplate))
	return buildAlert("system:pet:low_hp", "Pet: low HP", text, a, now)
}

// buffMissingAlertLocked checks the active character's watch list for the
// pet's kind. A watched spell counts as present when a live timer on the pet
// carries it, or a `/pet health` reply from the last minute lists it (a buff
// someone else cast has no timer). Newly missing spells fire one combined
// alert; each re-arms once seen on the pet again.
func (t *Tracker) buffMissingAlertLocked(prefs config.PetOverlaySettings, now time.Time) (trigger.TriggerFired, bool) {
	a := prefs.BuffMissing
	if !a.Enabled || t.st.Kind == KindUnknown || t.st.Name == "" ||
		now.Sub(t.startedAt) < buffMissingGrace || t.cfg.ActiveChar == nil {
		return trigger.TriggerFired{}, false
	}
	list := prefs.WatchLists[strings.ToLower(t.cfg.ActiveChar())]
	watched := list.Summoned
	if t.st.Kind == KindCharm {
		watched = list.Charm
	}
	if len(watched) == 0 {
		return trigger.TriggerFired{}, false
	}

	present := map[string]bool{}
	if t.cfg.PetTimers != nil {
		for _, s := range t.cfg.PetTimers(t.st.Name) {
			present[foldSpellName(s)] = true
		}
	}
	if h := t.st.Health; h != nil && now.Sub(h.At) <= healthFreshForWatch {
		for _, s := range h.Spells {
			present[foldSpellName(s.Name)] = true
		}
	}

	var newly []string
	for _, spell := range watched {
		f := foldSpellName(spell)
		if f == "" {
			continue
		}
		if present[f] {
			delete(t.alerts.missing, f)
			continue
		}
		if !t.alerts.missing[f] {
			t.alerts.missing[f] = true
			newly = append(newly, strings.TrimSpace(spell))
		}
	}
	if len(newly) == 0 {
		return trigger.TriggerFired{}, false
	}
	sort.Strings(newly)
	text := strings.NewReplacer(
		"{pet}", t.petLabelLocked(),
		"{spells}", strings.Join(newly, ", "),
	).Replace(orDefaultText(a.Template, config.DefaultPetBuffMissingTemplate))
	return buildAlert("system:pet:buff_missing", "Pet: buff missing", text, a, now)
}

// petLabelLocked is the pet's name for alert text.
func (t *Tracker) petLabelLocked() string {
	if t.st.Name != "" {
		return t.st.Name
	}
	return "Your pet"
}

// buildAlert turns an alert's action settings into a synthetic trigger fire.
// ok is false when every action is switched off.
func buildAlert(id, name, text string, a config.PetAlertSettings, now time.Time) (trigger.TriggerFired, bool) {
	var actions []trigger.Action
	if a.OverlayEnabled {
		actions = append(actions, trigger.Action{
			Type:         trigger.ActionOverlayText,
			Text:         text,
			DurationSecs: a.OverlayDurationSecs,
			Color:        a.OverlayColor,
		})
	}
	if a.TTSEnabled {
		actions = append(actions, trigger.Action{
			Type:   trigger.ActionTextToSpeech,
			Text:   text,
			Voice:  a.TTSVoice,
			Volume: float64(a.TTSVolume) / 100,
		})
	}
	if a.SoundEnabled && a.SoundPath != "" {
		actions = append(actions, trigger.Action{
			Type:      trigger.ActionPlaySound,
			SoundPath: a.SoundPath,
			Volume:    float64(a.SoundVolume) / 100,
		})
	}
	if len(actions) == 0 {
		return trigger.TriggerFired{}, false
	}
	return trigger.TriggerFired{
		TriggerID:   id,
		TriggerName: name,
		MatchedLine: text,
		Actions:     actions,
		FiredAt:     now,
	}, true
}

func orDefault(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

func orDefaultText(v, def string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return def
}
