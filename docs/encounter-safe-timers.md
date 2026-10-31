# Encounter-safe custom timers

Two optional settings extend the existing log-trigger/custom-timer owners. They do not enable a trigger, infer which raid is active, or turn a category into a zone filter.

- **Keep the first countdown** (`timer_if_idle`): while the same owned timer is active, another accepted start does not extend its expiry or replay its initial actions. An authoritative stop or natural expiry allows a new start. This is a timer-state admission policy, not a long refire cooldown.
- **Cancel on zone, character, or log change** (`timer_clear_on_context_change`): retire that opted-in timer when its context is lost. Ordinary spell timers and custom timers that did not opt in keep their existing behavior.

Both default to `false` when omitted. They are independent and apply only to log-source, nonstacking, nonrepeating custom timers. The API normalizes incompatible combinations off; the editor disables unavailable controls and sends `false` when saving an ineligible mode. Omitted update fields preserve saved values during existing bulk-toggle/category operations; explicit `false` clears an option.

## Example

A long event with one opening message should keep its original deadline, not refill when the opening is repeated:

```json
{
  "name": "Example event deadline timer",
  "enabled": false,
  "source": "log",
  "pattern": "^The example event begins\\.$",
  "characters": ["ExampleCharacter"],
  "actions": [],
  "timer_type": "custom",
  "timer_duration_secs": 600,
  "timer_stack": false,
  "timer_repeat": false,
  "timer_if_idle": true,
  "timer_clear_on_context_change": true,
  "worn_off_pattern": "^(?:The example event is complete|The example event has reset)\\.$",
  "spell_id": 0,
  "timer_alerts": []
}
```

This is a synthetic example. Replace the character and full start/end messages with verified messages from the intended event. Character selections are not zone restrictions. Player chat quotations, generic experience gains, common spells and loose boss-name matches are not adequate event authority.

A short warning rearmed by each genuine server announcement can use context cancellation without `timer_if_idle`. An event's long overall deadline should not be started again by later phases. Separate preparation and overall timers must each have complete cancellation messages, including later phases that prove preparation ended even if an earlier wave message was missed.

## Context and ownership

Opted-in timers belong to the triggering definition and timer identity/target. Retiring one must not cancel another trigger with the same display name, an automatic spell bar, or an ordinary custom timer. Harmless definition reloads do not extend the running epoch; disabling/deleting or materially changing its owner cannot leave an orphaned managed timer.

Context retirement covers zone changes in raw-line order, active-character changes, configured EQ/log source changes, physical replacement/truncation, parsing disable, replay ownership and tailer shutdown. A zone line followed by a legitimate opening in the same append must clear the old context before accepting the new opening—not clear the new timer later when parsed events are delivered.

No running timer is reconstructed from persisted definitions on restart. A missed initial message stays missed; a later phase is not permission to guess a fresh full duration. Source/context changes must not revive buffered old-context starts. No second watcher or helper process is needed.

## Compatibility and testing boundary

- Existing omitted/false definitions retain their legacy matching, duration, deduplication and cross-zone behavior.
- A sink without the opted-in capability must not silently fall back to unsafe legacy behavior.
- The silent trigger tester reports matching and selected timer policy with effects off; it does not execute or inspect a live timer latch. Test actual admission/expiry/cancellation through real trigger and timer owners in isolated state.
- Persistent API/SQLite roundtrips, old-schema defaults, copy/import behavior and editor save semantics are separate checks from real lifecycle behavior.
- Source-derived raid fixtures do not establish private-server deployment, local log reception, visible overlays or audible output. Those need separate native acceptance.

The feature does not change encounter scripts, add a global active-event gate, or authorize installing/enabling an alert pack. Configuration created for these settings requires a backend that implements them; successful submission to an older server that ignores unknown fields is not capability proof.
