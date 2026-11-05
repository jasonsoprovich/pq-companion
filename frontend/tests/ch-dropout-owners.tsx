// Actual shipping owners, designed synthetic feed, inert transports only.
import React from 'react'
import { createRoot } from 'react-dom/client'
import { flushSync } from 'react-dom'
import Panel from '../src/components/overlays/CHMetronomePanel'
import Popout from '../src/pages/CHMetronomeOverlayWindowPage'
import { useMetronomeAlerts } from '../src/hooks/useMetronomeAlerts'
import './ch-dropout-isolation'
const prior = JSON.parse(document.getElementById('result')!.textContent!)
const cases: any[] = prior.cases, base = 1800000000000
const record = (name: string, pass: boolean, observed: unknown, ...assertions: string[]) => cases.push({ name, pass, observed, assertions })
let now = base, nextInterval = 1, open = true
const intervals = new Map<number, { fn: () => void; ms: number; next: number }>()
const effects: any[] = []; (window as any).__effects = effects
const requests: any[] = [], sockets: FakeSocket[] = []
class FakeSocket {
  static OPEN = 1; static CONNECTING = 0; static CLOSING = 2
  readyState = 1; onmessage: any; onopen: any
  constructor(public url: string) { sockets.push(this); Promise.resolve().then(() => this.onopen?.()) }
  close() { this.readyState = 3 }
}
let config: any = { ch_chain: { secondary_enabled: true }, preferences: {
  metronome_start_alert: { enabled: true, type: 'play_sound', sound_path: 'inert.wav', volume: 50 },
  metronome_cast_alert: { enabled: true, type: 'overlay_text', text: 'CAST', duration_secs: 2 },
} }
let currentRows: any[] = []
async function settle() { for (let i = 0; i < 8; i++) await Promise.resolve(); await new Promise(r => setTimeout(r, 0)) }
function emit(type: string, data: any) { flushSync(() => sockets.forEach(s => s.onmessage?.({ data: JSON.stringify({ type, data }) }))) }
async function advance(at: number) {
  const end = base + at * 1000
  while (now < end) {
    now = Math.min(end, now + 100)
    flushSync(() => { for (const v of intervals.values()) if (now >= v.next) { v.next = now + v.ms; v.fn() } })
  }
  await settle()
}
function AlertOwner() { useMetronomeAlerts(); return null }
const root = createRoot(document.getElementById('root')!)
function mount(panel = true, popout = true, audio = true) { flushSync(() => root.render(<><section key="panel" id="panel">{panel && <Panel />}</section><section key="popout" id="popout">{popout && <Popout />}</section>{audio && <AlertOwner key="audio" />}</>)) }
function text(id: string) { return document.getElementById(id)?.textContent ?? '' }
async function ownerCases() {
  localStorage.clear(); now = base; Date.now = () => now
  window.setInterval = ((fn: () => void, ms: number) => { const id = nextInterval++; intervals.set(id, { fn, ms, next: now + ms }); return id }) as any
  window.clearInterval = ((id: number) => intervals.delete(id)) as any
  ;(window as any).WebSocket = FakeSocket
  ;(window as any).electron = { overlay: { popoutStates: async () => ({ chMetronome: open }), getLocked: async () => false } }
  window.fetch = (async (url: string, init: any) => {
    const path = new URL(url).pathname; requests.push({ path, method: init?.method ?? 'GET' })
    if (path === '/api/triggers/timer-alert-overlay') { effects.push({ kind: 'overlay', at: now, payload: JSON.parse(init.body) }); return new Response('{}') }
    if (path === '/api/config') return new Response(JSON.stringify(config))
    if (path === '/api/overlay/timers') return new Response(JSON.stringify({ timers: currentRows }))
    throw new Error(`Forbidden request ${path}`)
  }) as any
  localStorage.setItem('chMetronome:position', '1'); localStorage.setItem('chMetronome:chainSize', '5')
  mount(); await settle()
  const checks = [...document.querySelectorAll<HTMLInputElement>('input[type=checkbox]')]
  record('shipping-controls-default-off', checks.length === 2 && checks.every(c => !c.checked), checks.map(c => c.checked), 'actual controls unchecked')
  if (checks[0]) { flushSync(() => checks[0].click()); await settle() }
  // Baseline also receives future optional preference; behavioral RED, no missing exports.
  if (!checks.length) localStorage.setItem('chMetronome:sustainedDropout', 'true')
  record('shipping-toggle-persistence-and-sync', localStorage.getItem('chMetronome:sustainedDropout') === 'true' && checks.length === 2 && [...document.querySelectorAll<HTMLInputElement>('input[type=checkbox]')].every(c => c.checked), { saved: localStorage.getItem('chMetronome:sustainedDropout'), panel: text('panel'), popout: text('popout') }, 'panel click saves true and updates sibling')
  const calls: { slot: number; at: number }[] = []
  const send = async (slot: number, at: number) => {
    await advance(at); calls.push({ slot, at })
    currentRows = calls.filter(r => at - r.at < 10).map(r => ({ id: String(r.slot), category: 'ch_chain', target_name: 'Tank', spell_name: `#${r.slot} Tank ← ${r.slot === 1 ? 'You' : 'Cleric'}`, starts_at: new Date(base + r.at * 1000).toISOString(), remaining_seconds: 10 - (at - r.at) }))
    emit('overlay:timers', { timers: currentRows }); await settle()
  }
  for (let at = 0; at < 60; at += 4) await send(at / 4 % 5 + 1, at)
  for (let at = 60; at <= 136; at += 4) await send((at - 60) / 4 % 4 + 1, at)
  record('both-shipping-views-compressed', ['panel', 'popout'].every(id => text(id).includes('4.0s') && text(id).includes('Skipping #5') && text(id).includes('shortened rotation (4/5)')), { panel: text('panel'), popout: text('popout') }, 'rendered countdown four seconds, skipped slot visible')
  await advance(140)
  record('shipping-alert-fire-compressed', effects.some(e => e.kind === 'overlay' && e.at >= base + 139800 && e.at <= base + 140000), effects.filter(e => e.at >= base + 136000), 'shipping fire posts CAST at compressed time via inert fetch')
  const before = effects.length; await send(5, 140)
  record('both-views-rejoin', ['panel', 'popout'].every(id => !text(id).includes('Skipping') && text(id).includes('4.0s')), { panel: text('panel'), popout: text('popout') }, '#5 clears skipped render immediately')
  await advance(144)
  record('rejoin-no-double-cue', effects.length === before, effects.slice(before), 'rejoin cannot duplicate prior cues')
  localStorage.setItem('chMetronome:delay', '4.5')
  window.dispatchEvent(new StorageEvent('storage', { key: 'chMetronome:delay' })); await settle()
  record('adaptive-fractional-config-sync', ['panel', 'popout'].every(id => text(id).includes('4.5s')), { panel: text('panel'), popout: text('popout') }, 'storage delivery shares adaptive fractional value')
  const disable = document.querySelector<HTMLInputElement>('#popout input[type=checkbox]')
  if (disable) { flushSync(() => disable.click()); await settle() }
  const afterDisable = effects.length; await advance(145)
  record('disable-retires-pending-cue', localStorage.getItem('chMetronome:sustainedDropout') === 'false' && effects.length === afterDisable && ['panel', 'popout'].every(id => !text(id).includes('Skipping')), effects.slice(afterDisable), 'opt-out persists; obsolete cue silent')
  flushSync(() => root.render(null)); await settle(); currentRows = []; mount(); await settle()
  record('reload-persists-off', [...document.querySelectorAll<HTMLInputElement>('input[type=checkbox]')].length === 2 && [...document.querySelectorAll<HTMLInputElement>('input[type=checkbox]')].every(c => !c.checked), localStorage.getItem('chMetronome:sustainedDropout'), 'fresh owners load false')
  localStorage.setItem('chMetronome:alertsEnabled', 'false'); window.dispatchEvent(new StorageEvent('storage', { key: 'chMetronome:alertsEnabled' }))
  const muted = effects.length; await send(5, 160); await advance(164)
  record('mute-gate', effects.length === muted, effects.slice(muted), 'muted transports silent')
  localStorage.setItem('chMetronome:alertsEnabled', 'true'); window.dispatchEvent(new StorageEvent('storage', { key: 'chMetronome:alertsEnabled' }))
  await advance(165)
  record('unmute-no-catchup', effects.length === muted, effects.slice(muted), 'unmute does not replay consumed cue')
  open = false; await advance(168); const closed = effects.length; await send(5, 180); await advance(184)
  record('closed-overlay-gate', effects.length === closed, effects.slice(closed), 'existing poll gate prevents closed overlay alert')
  open = true; await advance(188)
  config.preferences.metronome_cast_alert = { enabled: true, type: 'text_to_speech', tts_template: 'Cast now', tts_volume: 70, voice: 'inert' }
  emit('config:updated', {}); await settle(); await send(5, 200); await advance(204)
  record('real-fire-sound-and-tts-transports', effects.some(e => e.kind === 'sound') && effects.some(e => e.kind === 'tts'), effects.filter(e => e.at >= base + 200000), 'shipping fire calls inert sound/TTS transports')
  // Exercise the shortened-cycle dedup threshold through the real alert hook,
  // not only the shared calculation (six configured slots, two survivors).
  const enableAgain = document.querySelector<HTMLInputElement>('#panel input[type=checkbox]')
  if (enableAgain) { flushSync(() => enableAgain.click()); await settle() }
  const increase = document.querySelector<HTMLButtonElement>('#panel button[title="Increase Clerics"]')
  if (increase) { flushSync(() => increase.click()); await settle() }
  calls.length = 0
  for (let at = 240; at < 312; at += 4) await send((at - 240) / 4 % 6 + 1, at)
  for (let at = 312; at <= 440; at += 4) await send((at - 312) / 4 % 2 + 1, at)
  const shortCues = effects.filter(e => e.kind === 'tts' && e.at >= base + 399800 && e.at <= base + 440000)
  record('shortened-cycle-alert-repeat', [400, 408, 416, 424, 432, 440].every(at => shortCues.filter(e => e.at >= base + at * 1000 - 200 && e.at <= base + at * 1000).length === 1) && shortCues.length === 6,
    { shortCues, panel: text('panel'), popout: text('popout') }, 'actual hook fires once each eight-second cycle, not configured 24-second cycle')
  const secondary = [...document.querySelectorAll<HTMLButtonElement>('#panel button')].find(b => b.textContent === 'Secondary')
  if (secondary) { flushSync(() => secondary.click()); await settle() }
  const afterSwitch = effects.length; await advance(444)
  record('shipping-chain-switch-retirement', effects.length === afterSwitch && ['panel', 'popout'].every(id => text(id).includes('secondary') && !text(id).includes('Skipping')),
    { panel: text('panel'), popout: text('popout'), effects: effects.slice(afterSwitch) }, 'actual chain control synchronizes and retires queued old-chain cue')
  const rampRow = (at: number) => ({ id: 'ramp1', category: 'ch_chain_2', target_name: 'Ramp', spell_name: '#1 Ramp ← You', starts_at: new Date(base + at * 1000).toISOString(), remaining_seconds: 10 })
  emit('overlay:timers', { timers: [rampRow(444)] }); await settle()
  await advance(480)
  emit('overlay:timers', { timers: [rampRow(480)] }); await settle()
  const staleStart = effects.length
  await advance(504)
  record('silent-feed-expires-projected-cue', !effects.slice(staleStart).some(e => e.kind === 'tts' || e.kind === 'overlay') && ['panel', 'popout'].every(id => text(id).includes('Waiting')),
    { panel: text('panel'), popout: text('popout'), effects: effects.slice(staleStart) }, 'no feed after real call: all owners expire long-range prediction before cast')
  flushSync(() => root.render(null)); await settle()
  record('owner-cleanup', intervals.size === 0, { intervals: intervals.size, socketCount: sockets.length }, 'unmount removes intervals')
}
async function freshConsumerCases() {
  const evidenceKey = 'chMetronome:liveEvidence'
  const dispatch = window.dispatchEvent.bind(window)
  let storageWire = false, holdReplies = false
  const messages: any[] = [], held: string[] = []
  window.dispatchEvent = ((event: Event) => {
    if (event.type !== evidenceKey) return dispatch(event)
    const raw = (event as CustomEvent).detail, message = JSON.parse(raw)
    messages.push(message)
    if (holdReplies && message.kind === 'response') { held.push(raw); return true }
    return dispatch(storageWire ? new StorageEvent('storage', { key: evidenceKey, newValue: raw }) : event)
  }) as typeof window.dispatchEvent
  const ready = async () => { for (let i = 0; i < 4; i++) await settle(); flushSync(() => {}) }
  for (const lane of ['fresh-popout', 'fresh-panel-audio', 'storage-fresh-popout', 'storage-fresh-panel-audio', 'stale-peer', 'new-target', 'config-retirement', 'retired-reply', 'stale-reply', 'return-before-reply']) {
    storageWire = lane.startsWith('storage-'); holdReplies = false; held.length = 0; messages.length = 0
    const freshAudio = lane.endsWith('panel-audio')
    localStorage.clear(); now = base; currentRows = []; effects.length = 0; open = true
    config.preferences.metronome_cast_alert = { enabled: true, type: 'overlay_text', text: 'CAST', duration_secs: 2 }
    localStorage.setItem('chMetronome:position', '1'); localStorage.setItem('chMetronome:chainSize', '5')
    localStorage.setItem('chMetronome:sustainedDropout', 'true')
    mount(!freshAudio, freshAudio, !freshAudio); await ready()
    const calls: { slot: number; at: number }[] = []
    const send = async (slot: number, at: number) => {
      await advance(at); calls.push({ slot, at })
      currentRows = calls.filter(r => at - r.at < 10).map(r => ({ id: String(r.slot), category: 'ch_chain', target_name: 'Tank', spell_name: `#${r.slot} Tank ← ${r.slot === 1 ? 'You' : 'Cleric'}`, starts_at: new Date(base + r.at * 1000).toISOString(), remaining_seconds: 10 - (at - r.at) }))
      emit('overlay:timers', { timers: currentRows }); await settle()
    }
    for (let at = 0; at < 60; at += 4) await send(at / 4 % 5 + 1, at)
    for (let at = 60; at <= 136; at += 4) await send((at - 60) / 4 % 4 + 1, at)
    if (lane === 'stale-peer') {
      await advance(151); currentRows = []; mount(); await ready()
      record(lane, ['panel', 'popout'].every(id => text(id).includes('Waiting') && !text(id).includes('Skipping')),
        { panel: text('panel'), popout: text('popout'), responses: messages.filter(m => m.kind === 'response') }, 'expired mature owner cannot seed fresh consumer')
    } else if (lane === 'new-target') {
      currentRows = [{ ...currentRows[currentRows.length - 1], target_name: 'Other', spell_name: '#1 Other ← You' }]
      mount(); await ready()
      const fresh = text('popout')
      emit('overlay:timers', { timers: currentRows }); await ready()
      record(lane, !fresh.includes('Skipping') && ['panel', 'popout'].every(id => !text(id).includes('Skipping')),
        { fresh, panel: text('panel'), popout: text('popout') }, 'same config but different own target rejects mature evidence; ordinary feed retires all owners')
    } else if (lane === 'stale-reply' || lane === 'return-before-reply') {
      holdReplies = true; mount(); await ready()
      const queued = [...held]
      if (lane === 'stale-reply') await advance(151)
      else await send(5, 140)
      const before = effects.length
      for (const raw of queued) dispatch(new StorageEvent('storage', { key: evidenceKey, newValue: raw }))
      await ready(); await advance(lane === 'stale-reply' ? 155 : 144)
      record(lane, queued.length > 0 && ['panel', 'popout'].every(id => !text(id).includes('Skipping')) && effects.length === before,
        { queued: queued.length, panel: text('panel'), popout: text('popout'), effects: effects.slice(before) }, 'same request expired or newer return observed: delayed compression rejected, no new cue')
    } else if (lane === 'config-retirement' || lane === 'retired-reply') {
      holdReplies = true; mount(); await ready()
      const queued = [...held]
      if (lane === 'config-retirement') {
        const increase = document.querySelector<HTMLButtonElement>('#panel button[title="Increase Clerics"]')!
        flushSync(() => increase.click()); await ready()
      } else {
        flushSync(() => root.render(null)); await ready(); mount(); await ready()
      }
      for (const raw of queued) dispatch(new StorageEvent('storage', { key: evidenceKey, newValue: raw }))
      await ready(); const before = effects.length; await advance(140)
      record(lane, queued.length > 0 && ['panel', 'popout'].every(id => !text(id).includes('Skipping')) && !effects.slice(before).some(e => e.kind === 'overlay'),
        { queued: queued.length, panel: text('panel'), popout: text('popout'), effects: effects.slice(before) }, 'delayed response cannot cross config retirement or unmount/remount nonce')
    } else {
      mount(); await ready()
      const mounted = { panel: text('panel'), popout: text('popout') }
      await advance(140)
      const fired = effects.filter(e => e.kind === 'overlay' && e.at >= base + 139800 && e.at <= base + 140000)
      record(`${lane}-countdown-and-real-fire`, Object.values(mounted).every(t => t.includes('4.0s') && t.includes('Skipping #5')) && fired.length === 1 && ['panel', 'popout'].every(id => text(id).includes('CAST NOW')),
        { mounted, fired, atFire: { panel: text('panel'), popout: text('popout') }, wire: storageWire ? 'simulated storage event transport' : 'local event', responses: messages.filter(m => m.kind === 'response').length }, 'fresh rendered owners agree with mature owner and shipping CAST at four seconds')
      const before = effects.length; await send(5, 140); await advance(144)
      record(`${lane}-ordinary-return`, ['panel', 'popout'].every(id => !text(id).includes('Skipping')) && effects.length === before,
        { panel: text('panel'), popout: text('popout'), effects: effects.slice(before) }, 'return restores all owners without duplicate cue')
      if (lane === 'fresh-popout') {
        const oldResponse = messages.find(m => m.kind === 'response')
        flushSync(() => root.render(null)); await ready()
        localStorage.setItem(evidenceKey, JSON.stringify(oldResponse ?? {}))
        localStorage.setItem('chMetronome:seen:main', JSON.stringify({ 1: now, 2: now, 3: now, 4: now, 5: now - 80000 }))
        mount(); await ready()
        dispatch(new StorageEvent('storage', { key: evidenceKey, newValue: JSON.stringify(oldResponse ?? {}) })); await ready()
        record('full-reload-rejects-persisted-and-replayed-evidence', ['panel', 'popout'].every(id => !text(id).includes('Skipping')),
          { panel: text('panel'), popout: text('popout') }, 'no live mature sibling: old response and legacy learned map cannot restore dropout')
      }
    }
    flushSync(() => root.render(null)); await ready()
  }
  window.dispatchEvent = dispatch
  record('fresh-owner-cleanup', intervals.size === 0, { intervals: intervals.size }, 'fresh and retired consumers release their tickers')
}
;(async () => {
  try { await ownerCases(); await freshConsumerCases() } catch (error) { record('exception', false, String(error), 'fixture completes') }
  document.getElementById('result')!.textContent = JSON.stringify({ status: cases.every(c => c.pass) ? 'PASS' : 'FAIL', cases, effects, requests, evidenceTier: 'synthetic shipping owners; not raid replay' })
})()
