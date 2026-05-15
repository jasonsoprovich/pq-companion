package zealpipe

import (
	"encoding/json"
	"fmt"
)

// Envelope is the outer JSON shape on every pipe message line. The Data field
// is left as a raw payload so callers decode only the variants they care about.
type Envelope struct {
	Type      PipeMessageType `json:"type"`
	Character string          `json:"character"`
	Data      json.RawMessage `json:"data"`
}

// Label is one entry inside a MsgLabel payload. Zeal sends label data as an
// array of these.
type Label struct {
	Type  LabelType       `json:"type"`
	Value string          `json:"value"`
	Meta  json.RawMessage `json:"meta,omitempty"`
}

// Gauge is one entry inside a MsgGauge payload.
type Gauge struct {
	Type  GaugeType `json:"type"`
	Value float64   `json:"value"`
	Text  string    `json:"text,omitempty"`
}

// Player is the per-tick player snapshot carried by MsgPlayer.
type Player struct {
	Zone       int      `json:"zone"`
	Location   Location `json:"location"`
	Heading    float64  `json:"heading"`
	AutoAttack bool     `json:"autoattack"`
}

// Location is a 3D world position.
type Location struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// PipeCmd carries a custom string sent via in-game /pipe <text>.
type PipeCmd struct {
	Text string `json:"text"`
}

// DecodeEnvelope parses a single JSON line. Returns an error if the bytes
// aren't valid JSON or don't fit the outer envelope shape.
func DecodeEnvelope(line []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return env, fmt.Errorf("zealpipe: decode envelope: %w", err)
	}
	return env, nil
}

// DecodeLabels parses a MsgLabel payload into the Label array. Returns nil
// on empty payloads rather than an error — Zeal occasionally emits messages
// with zero entries.
func DecodeLabels(payload json.RawMessage) ([]Label, error) {
	if len(payload) == 0 || string(payload) == "null" {
		return nil, nil
	}
	var labels []Label
	if err := json.Unmarshal(payload, &labels); err != nil {
		return nil, fmt.Errorf("zealpipe: decode labels: %w", err)
	}
	return labels, nil
}

// DecodeGauges parses a MsgGauge payload.
func DecodeGauges(payload json.RawMessage) ([]Gauge, error) {
	if len(payload) == 0 || string(payload) == "null" {
		return nil, nil
	}
	var gauges []Gauge
	if err := json.Unmarshal(payload, &gauges); err != nil {
		return nil, fmt.Errorf("zealpipe: decode gauges: %w", err)
	}
	return gauges, nil
}

// DecodePlayer parses a MsgPlayer payload.
func DecodePlayer(payload json.RawMessage) (Player, error) {
	var p Player
	if err := json.Unmarshal(payload, &p); err != nil {
		return p, fmt.Errorf("zealpipe: decode player: %w", err)
	}
	return p, nil
}

// DecodePipeCmd parses a MsgCmd payload.
func DecodePipeCmd(payload json.RawMessage) (PipeCmd, error) {
	var c PipeCmd
	if err := json.Unmarshal(payload, &c); err != nil {
		return c, fmt.Errorf("zealpipe: decode cmd: %w", err)
	}
	return c, nil
}
