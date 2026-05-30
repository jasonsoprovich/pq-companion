package respawn

// Quarm fast-respawn (the "RespawnReductionSystem"). On Project Quarm a zone's
// raw spawn2.respawntime is not always what players experience: zones flagged
// with reducedspawntimers collapse certain respawn ranges down to a fixed
// "fast" value. A netherbian drone, for example, stores a 1200s (20 min) base
// but actually repops in 8 min.
//
// The mapping below mirrors the server exactly — zone/spawn2.cpp::resetTimer()
// in the EQMacEmu/Quarm fork — and the bound constants are Quarm's
// rule_values (ruleset 1), quoted here in their native milliseconds:
//
//	Dungeon (zone.castdungeon = 1):
//	  base in [900000, 2400000] ms -> 480000 ms (8:00)
//	  base in [360001,  899000] ms -> 360000 ms (6:00)
//	Standard (zone.castdungeon = 0), only for newbie mobs (level 1..14):
//	  base in [ 60001,  900000] ms ->  60000 ms (1:00)
//	  base in [ 12001,   60000] ms ->  12000 ms (0:12)
//	Anything outside those ranges is left at its raw value.
//
// The server compares the timer in milliseconds, so we do too (base * 1000) to
// avoid an off-by-one at the boundaries (e.g. a 360s base must NOT fall in the
// 360001ms lower-bound range).
//
// LIMITATION: reduction is keyed purely off the static zone row. Raid/guild
// instances run with reduction disabled on the live server, but an instance is
// indistinguishable from its open-world counterpart here — the EQ log never
// marks a guild instance as "(Instanced)", and the Zeal pipe reports only the
// base zoneidnumber. So an instanced run of a reduced zone will still show the
// fast timer. There is currently no signal available to detect this.
const (
	dungeonHigherBoundMinMs = 900000
	dungeonHigherBoundMaxMs = 2400000
	dungeonHigherBoundMs    = 480000

	dungeonLowerBoundMinMs = 360001
	dungeonLowerBoundMaxMs = 899000
	dungeonLowerBoundMs    = 360000

	stdHigherBoundMinMs = 60001
	stdHigherBoundMaxMs = 900000
	stdHigherBoundMs    = 60000

	stdLowerBoundMinMs = 12001
	stdLowerBoundMaxMs = 60000
	stdLowerBoundMs    = 12000

	// newbieMaxLevel: standard (non-dungeon) reduced zones only fast-respawn
	// mobs below this level. The server check is "level != 0 && level < 15".
	newbieMaxLevel = 15
)

// reduceRespawnTime maps a raw spawn2.respawntime (seconds) to Quarm's actual
// respawn time (seconds) given the killed mob's level and the zone's reduction
// flags. When the zone does not reduce, or the base falls outside every
// reducible range, the base is returned unchanged.
func reduceRespawnTime(baseSeconds, level int, reduced, dungeon bool) int {
	if !reduced || baseSeconds <= 0 {
		return baseSeconds
	}
	ms := baseSeconds * 1000

	if dungeon {
		switch {
		case ms >= dungeonHigherBoundMinMs && ms <= dungeonHigherBoundMaxMs:
			return dungeonHigherBoundMs / 1000
		case ms >= dungeonLowerBoundMinMs && ms <= dungeonLowerBoundMaxMs:
			return dungeonLowerBoundMs / 1000
		}
		return baseSeconds
	}

	// Standard reduced zones only speed up newbie-level mobs.
	if level <= 0 || level >= newbieMaxLevel {
		return baseSeconds
	}
	switch {
	case ms >= stdHigherBoundMinMs && ms <= stdHigherBoundMaxMs:
		return stdHigherBoundMs / 1000
	case ms >= stdLowerBoundMinMs && ms <= stdLowerBoundMaxMs:
		return stdLowerBoundMs / 1000
	}
	return baseSeconds
}
