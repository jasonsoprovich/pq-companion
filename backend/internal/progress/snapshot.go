package progress

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trader"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

// untrainedTradeskillValue is the lower of the two "can never train this
// skill" sentinels Zeal exports (254 = race-locked, 255 = class-locked); see
// internal/api/characters.go's identical constant. Both are excluded from
// TradeskillTotal so a character's total isn't inflated by skills they can
// never raise.
const untrainedTradeskillValue = 254

// recorderInitialDelay defers the Recorder's first scan well past process
// startup, per project_startup_critical_path: nothing should do eager heavy
// work at boot, since the renderer blocks on the backend port opening.
const recorderInitialDelay = 90 * time.Second

// recorderPollInterval mirrors trader.capturePollInterval's reasoning: Quarmy
// exports only change on camp/logout (or a manual /outputfile), so this can
// be lazy.
const recorderPollInterval = 60 * time.Second

// Recorder polls each known character's Quarmy export and appends a coin/
// totals snapshot when it changes. Unlike the event journal, nothing here is
// backfillable — a Quarmy export only ever reflects the character's current
// state, so history starts accumulating from whenever this first runs.
type Recorder struct {
	cfgMgr    *config.Manager
	store     *Store
	charStore *character.Store

	mu      sync.Mutex
	lastMod map[string]time.Time // character -> mod time of last-seen Quarmy export
}

// NewRecorder constructs a Recorder. Call Start to begin polling.
func NewRecorder(cfgMgr *config.Manager, store *Store, charStore *character.Store) *Recorder {
	return &Recorder{cfgMgr: cfgMgr, store: store, charStore: charStore, lastMod: map[string]time.Time{}}
}

// Start runs the polling loop until ctx is cancelled. Run in a goroutine.
func (r *Recorder) Start(ctx context.Context) {
	if r == nil || r.store == nil || r.charStore == nil {
		return
	}
	timer := time.NewTimer(recorderInitialDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}

	r.scan()
	ticker := time.NewTicker(recorderPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.scan()
		}
	}
}

// scan captures a snapshot for every tracked character whose Quarmy export
// changed since the last scan.
func (r *Recorder) scan() {
	eqPath := r.cfgMgr.Get().EQPath
	if eqPath == "" {
		return
	}
	chars, err := r.charStore.List()
	if err != nil {
		slog.Warn("progress: list characters", "err", err)
		return
	}
	for _, c := range chars {
		r.captureCharacter(eqPath, c.Name)
	}
}

func (r *Recorder) captureCharacter(eqPath, name string) {
	quarmyPath := zeal.FindQuarmyFile(eqPath, name)
	if quarmyPath == "" {
		return
	}
	mt := zeal.ModTime(quarmyPath)
	if mt.IsZero() {
		return
	}
	r.mu.Lock()
	unchanged := mt.Equal(r.lastMod[name])
	r.mu.Unlock()
	if unchanged {
		return
	}
	r.mu.Lock()
	r.lastMod[name] = mt
	r.mu.Unlock()

	snap, ok := r.buildSnapshot(eqPath, name, mt)
	if !ok {
		return
	}
	latest, ok, err := r.store.LatestSnapshot(name)
	if err != nil {
		slog.Warn("progress: latest snapshot", "character", name, "err", err)
		return
	}
	if ok && latest.Fingerprint() == snap.Fingerprint() {
		return // export's mod time changed but the totals didn't
	}
	if _, err := r.store.AppendSnapshot(snap); err != nil {
		slog.Warn("progress: append snapshot", "character", name, "err", err)
	}
}

// buildSnapshot parses the character's current Quarmy export (level, AAs,
// tradeskills, coin) and spellbook export (spell count) into one totals row.
func (r *Recorder) buildSnapshot(eqPath, name string, takenAt time.Time) (Snapshot, bool) {
	quarmyPath := zeal.FindQuarmyFile(eqPath, name)
	data, err := zeal.ParseQuarmy(quarmyPath, name)
	if err != nil {
		slog.Warn("progress: parse quarmy", "character", name, "err", err)
		return Snapshot{}, false
	}

	aaRanks := 0
	for _, aa := range data.AAs {
		aaRanks += aa.Rank
	}
	tradeskillTotal := 0
	for _, ts := range data.Tradeskills {
		if ts.Value < untrainedTradeskillValue {
			tradeskillTotal += ts.Value
		}
	}

	// Coin isn't in the Quarmy export's own struct — trader.ParseSnapshot
	// re-reads the same tab-delimited inventory section for the coin rows
	// (see its doc comment: it accepts either an -Inventory.txt or a
	// -Quarmy.txt export). zeal.ParseInventory can't be reused here: its
	// parseInventoryLine coerces count==0 to 1, which corrupts coin rows.
	var copper int64
	if snap, err := trader.ParseSnapshot(quarmyPath, name); err == nil {
		copper = snap.OnPersonCopper + snap.BankCopper
	}

	spellsKnown := 0
	if sbPath := zeal.FindSpellbookFile(eqPath, name); sbPath != "" {
		if sb, err := zeal.ParseSpellbook(sbPath, name); err == nil {
			spellsKnown = len(sb.SpellIDs)
		}
	}

	return Snapshot{
		Character:       name,
		TakenAt:         takenAt,
		Level:           data.Level,
		AARanks:         aaRanks,
		TradeskillTotal: tradeskillTotal,
		SpellsKnown:     spellsKnown,
		Copper:          copper,
	}, true
}
