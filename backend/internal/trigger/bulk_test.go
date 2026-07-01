package trigger

import "testing"

func TestActionTemplates_CRUDAndSingleDefault(t *testing.T) {
	s := openTestStore(t)

	a := &ActionTemplate{
		Name:      "Sound Alert",
		Actions:   []Action{{Type: ActionPlaySound, SoundPath: "/tmp/ding.mp3", Volume: 0.8}},
		IsDefault: true,
	}
	if err := s.CreateActionTemplate(a); err != nil {
		t.Fatalf("create: %v", err)
	}
	b := &ActionTemplate{
		Name:      "Big Overlay",
		Actions:   []Action{{Type: ActionOverlayText, Text: "ALERT", DurationSecs: 5}},
		IsDefault: true, // must steal the default flag from a
	}
	if err := s.CreateActionTemplate(b); err != nil {
		t.Fatalf("create: %v", err)
	}

	list, err := s.ListActionTemplates()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	defaults := 0
	for _, tmpl := range list {
		if tmpl.IsDefault {
			defaults++
			if tmpl.ID != b.ID {
				t.Fatalf("default = %s, want %s", tmpl.Name, b.Name)
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("defaults = %d, want exactly 1", defaults)
	}

	// Update A to become default; B loses it.
	a.IsDefault = true
	a.Name = "Sound Alert v2"
	if err := s.UpdateActionTemplate(a); err != nil {
		t.Fatalf("update: %v", err)
	}
	gotB, err := s.GetActionTemplate(b.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if gotB.IsDefault {
		t.Fatal("B should have lost the default flag")
	}

	if err := s.DeleteActionTemplate(a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetActionTemplate(a.ID); err != ErrTemplateNotFound {
		t.Fatalf("get deleted = %v, want ErrTemplateNotFound", err)
	}
}

func TestBulkApplyActions_ReplacesActions(t *testing.T) {
	s := openTestStore(t)
	t1 := makeTrigger("one", "Cat")
	t2 := makeTrigger("two", "Cat")
	for _, tr := range []*Trigger{t1, t2} {
		if err := s.Insert(tr); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	tmpl := []Action{{Type: ActionPlaySound, SoundPath: "/tmp/ding.mp3", Volume: 0.5}}
	res, err := BulkApplyActions(s, []string{t1.ID, t2.ID, "missing"}, tmpl)
	if err != nil {
		t.Fatalf("bulk apply: %v", err)
	}
	if res.Updated != 2 || res.Skipped != 1 {
		t.Fatalf("updated/skipped = %d/%d, want 2/1", res.Updated, res.Skipped)
	}
	got, err := s.Get(t1.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Actions) != 1 || got.Actions[0].SoundPath != "/tmp/ding.mp3" {
		t.Fatalf("actions = %+v, want template applied", got.Actions)
	}
}

func TestBulkConvertTTSToSound(t *testing.T) {
	s := openTestStore(t)

	// Trigger with a TTS action + an overlay action + a TTS timer alert.
	tts := makeTrigger("tts", "Cat")
	tts.Actions = []Action{
		{Type: ActionOverlayText, Text: "CHARM BROKE!", DurationSecs: 6},
		{Type: ActionTextToSpeech, Text: "Charm broke", Volume: 1.0},
	}
	tts.TimerAlerts = []TimerAlert{
		{ID: "fade", Seconds: 60, Type: TimerAlertTypeTextToSpeech, TTSTemplate: "{spell} fading soon", TTSVolume: 100},
	}
	// Trigger with no TTS anywhere — must count as skipped.
	plain := makeTrigger("plain", "Cat")
	for _, tr := range []*Trigger{tts, plain} {
		if err := s.Insert(tr); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	res, err := BulkConvertTTSToSound(s, []string{tts.ID, plain.ID}, "/tmp/fade.wav", 0.7, true)
	if err != nil {
		t.Fatalf("bulk convert: %v", err)
	}
	if res.Updated != 1 || res.Skipped != 1 {
		t.Fatalf("updated/skipped = %d/%d, want 1/1", res.Updated, res.Skipped)
	}

	got, err := s.Get(tts.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Actions[0].Type != ActionOverlayText {
		t.Fatalf("overlay action touched: %+v", got.Actions[0])
	}
	if got.Actions[1].Type != ActionPlaySound || got.Actions[1].SoundPath != "/tmp/fade.wav" || got.Actions[1].Volume != 0.7 {
		t.Fatalf("TTS action not converted: %+v", got.Actions[1])
	}
	if got.Actions[1].Text != "Charm broke" {
		t.Fatal("TTS text should be kept for round-tripping in the editor")
	}
	alert := got.TimerAlerts[0]
	if alert.Type != TimerAlertTypePlaySound || alert.SoundPath != "/tmp/fade.wav" || alert.Volume != 70 {
		t.Fatalf("timer alert not converted: %+v", alert)
	}
	if alert.Seconds != 60 {
		t.Fatalf("alert threshold changed: %v", alert.Seconds)
	}
}

func TestBulkConvertTTSToSound_TimerAlertsOptOut(t *testing.T) {
	s := openTestStore(t)
	tr := makeTrigger("tts", "Cat")
	tr.Actions = []Action{{Type: ActionTextToSpeech, Text: "hi"}}
	tr.TimerAlerts = []TimerAlert{
		{ID: "fade", Seconds: 60, Type: TimerAlertTypeTextToSpeech, TTSTemplate: "{spell} fading", TTSVolume: 100},
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := BulkConvertTTSToSound(s, []string{tr.ID}, "/tmp/fade.wav", 1.0, false); err != nil {
		t.Fatalf("bulk convert: %v", err)
	}
	got, err := s.Get(tr.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TimerAlerts[0].Type != TimerAlertTypeTextToSpeech {
		t.Fatal("timer alert converted despite opt-out")
	}
}
