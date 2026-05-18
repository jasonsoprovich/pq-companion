package zealpipe

import (
	"encoding/json"
	"fmt"
)

// Envelope is the outer JSON shape on every pipe message line.
//
// Zeal's serializer (Zeal/named_pipe.h:21) wraps each payload as a
// JSON-encoded *string*, not embedded JSON — i.e. the wire format is
//
//	{"type":1,"data_len":42,"data":"[{\"type\":28,\"value\":\"a gnoll\"}]","character":"Osui"}
//
// so callers must do a second json.Unmarshal on Data to get the typed payload.
// The Decode* helpers in this file encapsulate that pattern.
type Envelope struct {
	Type      PipeMessageType `json:"type"`
	Character string          `json:"character"`
	DataLen   int             `json:"data_len,omitempty"`
	Data      string          `json:"data"`
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
func DecodeLabels(payload string) ([]Label, error) {
	if payload == "" || payload == "null" {
		return nil, nil
	}
	var labels []Label
	if err := json.Unmarshal([]byte(payload), &labels); err != nil {
		return nil, fmt.Errorf("zealpipe: decode labels: %w", err)
	}
	return labels, nil
}

// DecodeGauges parses a MsgGauge payload.
func DecodeGauges(payload string) ([]Gauge, error) {
	if payload == "" || payload == "null" {
		return nil, nil
	}
	var gauges []Gauge
	if err := json.Unmarshal([]byte(payload), &gauges); err != nil {
		return nil, fmt.Errorf("zealpipe: decode gauges: %w", err)
	}
	return gauges, nil
}

// DecodePlayer parses a MsgPlayer payload.
func DecodePlayer(payload string) (Player, error) {
	var p Player
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return p, fmt.Errorf("zealpipe: decode player: %w", err)
	}
	return p, nil
}

// DecodePipeCmd parses a MsgCmd payload.
func DecodePipeCmd(payload string) (PipeCmd, error) {
	var c PipeCmd
	if err := json.Unmarshal([]byte(payload), &c); err != nil {
		return c, fmt.Errorf("zealpipe: decode cmd: %w", err)
	}
	return c, nil
}
