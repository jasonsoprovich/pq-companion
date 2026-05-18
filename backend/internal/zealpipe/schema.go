// Package zealpipe consumes the named-pipe IPC exposed by the Zeal mod
// (https://github.com/CoastalRedwood/Zeal) for the EverQuest Project Quarm
// client.
//
// Zeal publishes line-delimited JSON to a Windows named pipe named
// "\\.\pipe\zeal_<PID>" where <PID> is the running eqgame.exe process. Each
// line is one envelope:
//
//	{ "type": <int>, "character": "<charname>", "data": <payload> }
//
// We treat the schema as best-effort — Zeal is upstream-developed and IDs can
// shift between releases. Unknown enum values are ignored rather than rejected.
//
// Verified against Zeal HEAD on 2026-05-15. Canonical schema source:
// https://github.com/OkieDan/ZealPipes/blob/master/ZealPipes.Common/Enums.cs
package zealpipe

// PipeMessageType is the top-level "type" tag on every envelope. Values match
// Zeal's C++ pipe_data_type enum ordering (named_pipe.h).
type PipeMessageType int

const (
	MsgLog    PipeMessageType = 0 // log line entry
	MsgLabel  PipeMessageType = 1 // array of Label values
	MsgGauge  PipeMessageType = 2 // array of Gauge values
	MsgPlayer PipeMessageType = 3 // player state snapshot
	MsgCmd    PipeMessageType = 4 // custom string sent via in-game /pipe
	MsgRaid   PipeMessageType = 5 // raid roster
	MsgGroup  PipeMessageType = 6 // group roster
)

// String returns a human-readable name for logging.
func (t PipeMessageType) String() string {
	switch t {
	case MsgLog:
		return "log"
	case MsgLabel:
		return "label"
	case MsgGauge:
		return "gauge"
	case MsgPlayer:
		return "player"
	case MsgCmd:
		return "cmd"
	case MsgRaid:
		return "raid"
	case MsgGroup:
		return "group"
	default:
		return "unknown"
	}
}

// LabelType identifies a single field inside a Label-typed payload. Only the
// IDs we currently consume are listed — Zeal exposes 130+ but the rest are
// safely ignored at decode time.
type LabelType int

const (
	LabelCurrentHP        LabelType = 17
	LabelMaxHP            LabelType = 18
	LabelTargetName       LabelType = 28
	LabelTargetHPPerc     LabelType = 29
	LabelGroupMember1Name LabelType = 30
	LabelGroupMember5Name LabelType = 34
	LabelGroupMember1HP   LabelType = 35
	LabelGroupMember5HP   LabelType = 39
	LabelBuff0            LabelType = 45
	LabelBuff14           LabelType = 59
	LabelSpellSlot0       LabelType = 60
	LabelSpellSlot7       LabelType = 67
	LabelPlayerPetName    LabelType = 68
	LabelPlayerPetHPPerc  LabelType = 69
	LabelTargetPetOwner   LabelType = 82
	LabelMana             LabelType = 124
	LabelMaxMana          LabelType = 125
	LabelCastingName      LabelType = 134
	LabelBuff15           LabelType = 135
	LabelBuff20           LabelType = 140
)

// GaugeType identifies a numeric/timer gauge value. Same partial-coverage
// policy as LabelType.
type GaugeType int

const (
	GaugeHP            GaugeType = 1
	GaugeMana          GaugeType = 2
	GaugeStamina       GaugeType = 3
	GaugeExperience    GaugeType = 4
	GaugeAltExp        GaugeType = 5
	GaugeTarget        GaugeType = 6
	GaugeCasting       GaugeType = 7
	GaugeBreath        GaugeType = 8
	GaugeMemorize      GaugeType = 9
	GaugeScribe        GaugeType = 10
	GaugeGroup1HP      GaugeType = 11
	GaugeGroup5HP      GaugeType = 15
	GaugePetHP         GaugeType = 16
	GaugeServerTick    GaugeType = 24 // sync point for buff/spell timing
	GaugeCastRecovery  GaugeType = 25
	GaugeSpellRecast0  GaugeType = 26
	GaugeSpellRecast7  GaugeType = 33
	GaugeAttackRecover GaugeType = 34
	GaugeAAExp         GaugeType = 35
)
