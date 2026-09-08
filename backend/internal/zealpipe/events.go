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
//
// SpawnID/TargetID/PetID were added in Zeal v1.4.6 (PR #229). They are
// pointers because Zeal *omits* target_id/pet_id entirely when there is no
// target / no pet (rather than sending 0 or -1), and because every field is
// absent on any pre-1.4.6 Zeal — a nil pointer means "this Zeal build doesn't
// report it, or there genuinely isn't one", and consumers must fall back to
// the name-based path in both cases.
type Player struct {
	Zone       int      `json:"zone"`
	Location   Location `json:"location"`
	Heading    float64  `json:"heading"`
	AutoAttack bool     `json:"autoattack"`
	SpawnID    *int     `json:"spawn_id,omitempty"`
	TargetID   *int     `json:"target_id,omitempty"`
	PetID      *int     `json:"pet_id,omitempty"`
}

// RaidMember is one entry in a MsgRaid (type 5) payload. Zeal emits this
// array whenever the client is in a raid; we drop the whole message today.
//
// name/level/class/group/rank are always present. spawn_id/loc/heading are
// present only for members currently in your zone (Zeal resolves them through
// the entity manager). hp_current/hp_max/zone_id are present only for in-zone
// members AND only when the user has run "/pipe verbose on" (PipeVerbose,
// which defaults off). SpawnID was added in v1.4.6.
type RaidMember struct {
	Name    string    `json:"name"`
	Level   int       `json:"level"`
	Class   int       `json:"class"`
	Group   string    `json:"group"` // "0" = ungrouped, "1".."12"
	Rank    string    `json:"rank"`  // "Raid Leader" | "Group Leader" | ""
	SpawnID *int      `json:"spawn_id,omitempty"`
	Loc     *Location `json:"loc,omitempty"`
	Heading *float64  `json:"heading,omitempty"`
	HPCur   *int      `json:"hp_current,omitempty"` // PipeVerbose only
	HPMax   *int      `json:"hp_max,omitempty"`     // PipeVerbose only
	ZoneID  *int      `json:"zone_id,omitempty"`    // PipeVerbose only
}

// GroupMember is one entry in a MsgGroup (type 6) payload. name/spawn_id/
// loc/heading are always present; hp_current/hp_max/class/level/zone_id are
// PipeVerbose-only. SpawnID was added in v1.4.6.
type GroupMember struct {
	Name    string    `json:"name"`
	SpawnID *int      `json:"spawn_id,omitempty"`
	Loc     *Location `json:"loc,omitempty"`
	Heading *float64  `json:"heading,omitempty"`
	HPCur   *int      `json:"hp_current,omitempty"` // PipeVerbose only
	HPMax   *int      `json:"hp_max,omitempty"`     // PipeVerbose only
	Class   *int      `json:"class,omitempty"`      // PipeVerbose only
	Level   *int      `json:"level,omitempty"`      // PipeVerbose only
	ZoneID  *int      `json:"zone_id,omitempty"`    // PipeVerbose only
}

// Location is a 3D world position, with Zeal's field names.
//
// Read GameX/GameY rather than these fields directly — see the methods below.
type Location struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// GameX and GameY return the position in the convention quarm.db uses for
// spawn2.x / spawn2.y, which is what every other coordinate in this codebase
// means by x and y.
//
// Zeal reports position in EQ's internal struct order, which is (y, x, z) —
// the same order /loc prints, and the same order zealMap.ts already encodes for
// `/map marker`. So the JSON key "x" carries the game's Y and vice versa. Using
// the fields positionally transposes the coordinates, which is subtle rather
// than obvious: the values stay in range and track the player correctly, they
// are simply reflected about the diagonal.
//
// MEASURED 2026-07-30, standing in the Bazaar at /loc -784, -102: the arrow
// drew at map (803, 108) where the transposed reading predicts (784, 102) and
// the direct reading predicts (102, 784) — 43% of a map-width outside the
// zone's right edge. Netherbian Lair agreed.
func (l Location) GameX() float64 { return l.Y }
func (l Location) GameY() float64 { return l.X }

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

// DecodeRaid parses a MsgRaid (type 5) payload — an array of RaidMember.
// Returns nil (not an error) for an empty/null payload, matching DecodeLabels.
func DecodeRaid(payload string) ([]RaidMember, error) {
	if payload == "" || payload == "null" {
		return nil, nil
	}
	var members []RaidMember
	if err := json.Unmarshal([]byte(payload), &members); err != nil {
		return nil, fmt.Errorf("zealpipe: decode raid: %w", err)
	}
	return members, nil
}

// DecodeGroup parses a MsgGroup (type 6) payload — an array of GroupMember.
func DecodeGroup(payload string) ([]GroupMember, error) {
	if payload == "" || payload == "null" {
		return nil, nil
	}
	var members []GroupMember
	if err := json.Unmarshal([]byte(payload), &members); err != nil {
		return nil, fmt.Errorf("zealpipe: decode group: %w", err)
	}
	return members, nil
}
