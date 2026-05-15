package zealpipe

import (
	"testing"
)

func TestDecodeEnvelopeLabel(t *testing.T) {
	line := []byte(`{"type":1,"character":"Osui","data":[{"type":28,"value":"a gnoll pup"},{"type":29,"value":"73"}]}`)
	env, err := DecodeEnvelope(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Type != MsgLabel {
		t.Fatalf("Type = %v, want MsgLabel", env.Type)
	}
	if env.Character != "Osui" {
		t.Fatalf("Character = %q, want Osui", env.Character)
	}
	labels, err := DecodeLabels(env.Data)
	if err != nil {
		t.Fatalf("decode labels: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("len(labels) = %d, want 2", len(labels))
	}
	if labels[0].Type != LabelTargetName || labels[0].Value != "a gnoll pup" {
		t.Errorf("label[0] = %+v", labels[0])
	}
	if labels[1].Type != LabelTargetHPPerc || labels[1].Value != "73" {
		t.Errorf("label[1] = %+v", labels[1])
	}
}

func TestDecodeEnvelopePlayer(t *testing.T) {
	line := []byte(`{"type":3,"character":"Nariana","data":{"zone":24,"location":{"x":1.5,"y":-200,"z":3.0},"heading":128,"autoattack":true}}`)
	env, err := DecodeEnvelope(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Type != MsgPlayer {
		t.Fatalf("Type = %v, want MsgPlayer", env.Type)
	}
	p, err := DecodePlayer(env.Data)
	if err != nil {
		t.Fatalf("decode player: %v", err)
	}
	if p.Zone != 24 || !p.AutoAttack || p.Location.Y != -200 {
		t.Errorf("player = %+v", p)
	}
}

func TestDecodeEnvelopePipeCmd(t *testing.T) {
	line := []byte(`{"type":4,"character":"Osui","data":{"text":"pull"}}`)
	env, err := DecodeEnvelope(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Type != MsgCmd {
		t.Fatalf("Type = %v, want MsgCmd", env.Type)
	}
	c, err := DecodePipeCmd(env.Data)
	if err != nil {
		t.Fatalf("decode cmd: %v", err)
	}
	if c.Text != "pull" {
		t.Errorf("cmd text = %q, want pull", c.Text)
	}
}

func TestDecodeLabelsEmpty(t *testing.T) {
	labels, err := DecodeLabels(nil)
	if err != nil || labels != nil {
		t.Errorf("nil payload: labels=%v err=%v", labels, err)
	}
	labels, err = DecodeLabels([]byte("null"))
	if err != nil || labels != nil {
		t.Errorf("null payload: labels=%v err=%v", labels, err)
	}
}

func TestDecodeEnvelopeInvalid(t *testing.T) {
	if _, err := DecodeEnvelope([]byte("not json")); err == nil {
		t.Error("expected error for non-JSON")
	}
}

func TestUnknownLabelTypeAccepted(t *testing.T) {
	// Schema is best-effort — unknown IDs decode as their numeric value rather
	// than fail. Consumers ignore via a switch default.
	line := []byte(`{"type":1,"character":"X","data":[{"type":9999,"value":"x"}]}`)
	env, err := DecodeEnvelope(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	labels, err := DecodeLabels(env.Data)
	if err != nil {
		t.Fatalf("decode labels: %v", err)
	}
	if labels[0].Type != LabelType(9999) {
		t.Errorf("unknown label type lost: %v", labels[0].Type)
	}
}
