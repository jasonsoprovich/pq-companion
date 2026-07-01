package trigger

import "fmt"

// Bulk action edits
//
// The pain these solve: prebuilt packs ship TTS alerts, but many users
// prefer sound files — and re-pointing every trigger by hand is tedious.
// BulkApplyActions stamps a template's actions onto many triggers at once;
// BulkConvertTTSToSound swaps text_to_speech actions (and optionally the
// "fading soon" timer alerts) for a chosen sound file while leaving every
// other action untouched. Combined with preserve-mode pack updates, a sound
// scheme set up once survives future pack releases.

// BulkResult reports what a bulk edit touched. Skipped counts triggers that
// were selected but had nothing to change (e.g. no TTS action to convert).
type BulkResult struct {
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

// BulkApplyActions replaces the Actions list of every trigger in ids with a
// copy of actions. Unknown ids are skipped.
func BulkApplyActions(store *Store, ids []string, actions []Action) (*BulkResult, error) {
	if actions == nil {
		actions = []Action{}
	}
	res := &BulkResult{}
	for _, id := range ids {
		t, err := store.Get(id)
		if err == ErrNotFound {
			res.Skipped++
			continue
		}
		if err != nil {
			return res, err
		}
		t.Actions = append([]Action(nil), actions...)
		if err := store.Update(t); err != nil {
			return res, fmt.Errorf("bulk apply to %s: %w", id, err)
		}
		res.Updated++
	}
	return res, nil
}

// BulkConvertTTSToSound rewrites every text_to_speech action on the given
// triggers to play_sound with the supplied file and volume (0.0–1.0). Other
// actions are left alone. When includeTimerAlerts is set, TTS "fading soon"
// timer alerts convert too (their volume scale is 0–100). Triggers with no
// TTS anywhere are counted as skipped, not updated.
func BulkConvertTTSToSound(store *Store, ids []string, soundPath string, volume float64, includeTimerAlerts bool) (*BulkResult, error) {
	if soundPath == "" {
		return nil, fmt.Errorf("sound path is required")
	}
	if volume <= 0 || volume > 1 {
		volume = 1.0
	}
	res := &BulkResult{}
	for _, id := range ids {
		t, err := store.Get(id)
		if err == ErrNotFound {
			res.Skipped++
			continue
		}
		if err != nil {
			return res, err
		}
		changed := false
		for i := range t.Actions {
			if t.Actions[i].Type != ActionTextToSpeech {
				continue
			}
			// Keep Text/Voice so flipping the action type back in the editor
			// restores the old TTS (mirrors the editor's own spread behavior).
			t.Actions[i].Type = ActionPlaySound
			t.Actions[i].SoundPath = soundPath
			t.Actions[i].Volume = volume
			changed = true
		}
		if includeTimerAlerts {
			for i := range t.TimerAlerts {
				if t.TimerAlerts[i].Type != TimerAlertTypeTextToSpeech {
					continue
				}
				t.TimerAlerts[i].Type = TimerAlertTypePlaySound
				t.TimerAlerts[i].SoundPath = soundPath
				t.TimerAlerts[i].Volume = int(volume * 100)
				changed = true
			}
		}
		if !changed {
			res.Skipped++
			continue
		}
		if err := store.Update(t); err != nil {
			return res, fmt.Errorf("bulk convert %s: %w", id, err)
		}
		res.Updated++
	}
	return res, nil
}
