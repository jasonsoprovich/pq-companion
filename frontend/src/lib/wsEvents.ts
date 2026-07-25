// Centralized WebSocket event-type constants. Mirrors the named constants
// defined alongside each backend feature package (e.g. WSEventCombat in
// internal/combat/models.go). Keep these in sync with the backend — adding
// a new event means adding an entry both here and in the broadcasting
// package on the Go side.
export const WSEvent = {
  OverlayCombat: 'overlay:combat',
  OverlayTimers: 'overlay:timers',
  OverlayRespawns: 'overlay:respawns',
  OverlayRolls: 'overlay:rolls',
  OverlayFactions: 'overlay:factions',
  OverlayNPCTarget: 'overlay:npc_target',
  OverlayThreat: 'overlay:threat',
  OverlayRaidThreat: 'overlay:raidthreat',
  TriggerFired: 'trigger:fired',
  TriggerTest: 'trigger:test',
  TriggerTestPosition: 'trigger:test_position',
  TriggerTestSessionEnded: 'trigger:test_session_ended',
  ConfigUpdated: 'config:updated',
  ConfigCharacterDetected: 'config:character_detected',
  ChatNew: 'chat:new',
  LootNew: 'loot:new',
  SkillsUpdate: 'skills:update',
  BackfillProgress: 'backfill:progress',
  WishlistChanged: 'wishlist:changed',
  ZealConnected: 'zeal:connected',
  ZealDisconnected: 'zeal:disconnected',
  // Broadcast by the zeal watcher when a character's quarmy export is
  // (re)parsed on /camp or /outputfile — carries fresh stats/AAs/tradeskills.
  ZealQuarmy: 'zeal:quarmy',
  // Broadcast when the local player begins casting a recognized CH-chain
  // heal spell — lets the CH Metronome show a confirmed "cast sent" state
  // instead of assuming it once the countdown elapses.
  ChMetronomeSelfCast: 'ch_metronome:self_cast',
} as const

export type WSEventType = (typeof WSEvent)[keyof typeof WSEvent]
