// Centralized WebSocket event-type constants. Mirrors the named constants
// defined alongside each backend feature package (e.g. WSEventCombat in
// internal/combat/models.go). Keep these in sync with the backend — adding
// a new event means adding an entry both here and in the broadcasting
// package on the Go side.
export const WSEvent = {
  OverlayCombat: 'overlay:combat',
  OverlayTimers: 'overlay:timers',
  OverlayRolls: 'overlay:rolls',
  OverlayNPCTarget: 'overlay:npc_target',
  TriggerFired: 'trigger:fired',
  TriggerTest: 'trigger:test',
  TriggerTestPosition: 'trigger:test_position',
  TriggerTestSessionEnded: 'trigger:test_session_ended',
  ConfigUpdated: 'config:updated',
  ConfigCharacterDetected: 'config:character_detected',
} as const

export type WSEventType = (typeof WSEvent)[keyof typeof WSEvent]
