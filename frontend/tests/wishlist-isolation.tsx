// Actual shipping page, tabs, dialogs, API serialization and websocket hook.
// Only the transport is synthetic; no production state or handlers are injected.
import React from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import WishlistPage from '../src/pages/WishlistPage'

const characters = [{ id: 1, name: 'FixtureAlpha' }, { id: 2, name: 'FixtureBeta' }]
const item = { id: 33, name: 'ADDED TARGET', icon: 0, slots: 4, item_class: 0, item_type: 0 }
const list = (id: number, name = id === 1 ? 'ALPHA TARGET' : 'BETA TARGET') => ({
  entries: [{ id, character_id: id, item_id: id, slot_bucket: id === 1 ? 'Head' : 'Feet', sort_order: 0, keep_after_loot: false, item: { id, name, icon: 0, slots: 4 } }],
  slot_layout: [{ slot_bucket: id === 1 ? 'Head' : 'Feet', position: 0, collapsed: false }],
})
type Event = { path: string; method: string; body: any }
type Hold = { path: string; method: string; resolve?: (r: Response) => void }
let events: Event[] = [], holds: Hold[] = [], lists: Record<number, any> = {}
const errors: string[] = []
const response = (body: any, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
globalThis.fetch = async (url, init = {}) => {
  const path = new URL(String(url), 'http://fixture.invalid').pathname
  const method = init.method || 'GET'
  const body = init.body ? JSON.parse(String(init.body)) : null
  events.push({ path, method, body })
  const held = holds.find(h => h.path === path && h.method === method && !h.resolve)
  if (held) return new Promise<Response>(resolve => { held.resolve = resolve })
  if (path === '/api/characters') return response({ characters })
  if (path === '/api/piper/status') return response({ enabled: false, exe_found: false, model_found: false, model_config_found: false })
  if (path === '/api/kokoro/status') return response({ enabled: false, reachable: false, voices: [] })
  if (path === '/api/items') return response({ items: [item], total: 1 })
  if (path.endsWith('/sources')) return response({ drops: [], merchants: [], forage_zones: [], ground_spawns: [], tradeskills: [] })
  const m = path.match(/^\/api\/characters\/(\d+)\/wishlist(?:\/(.*))?$/)
  if (m) {
    const id = Number(m[1])
    if (method === 'GET') return response(lists[id])
    if (method === 'POST') {
      lists[id].entries.push({ id: 33, character_id: id, item_id: 33, item, slot_bucket: 'Head', sort_order: 1, keep_after_loot: false })
      return response({ entries: lists[id].entries })
    }
    if (method === 'DELETE') lists[id].entries = lists[id].entries.filter((e: any) => e.id !== Number(m[2]))
    if (method === 'PUT' && m[2] === 'slot-layout') lists[id].slot_layout = body.layout
    if (method === 'PATCH') lists[id].entries.find((e: any) => e.id === Number(m[2])).keep_after_loot = body.keep_after_loot
    return new Response(null, { status: 204 })
  }
  errors.push('Unexpected fetch: ' + method + ' ' + path)
  return response({ error: 'Unprovided fixture endpoint ' + path }, 404)
}
class InertWebSocket {
  static OPEN = 1; static CONNECTING = 0; static CLOSING = 2
  static sockets: InertWebSocket[] = []
  readyState = 1
  onmessage?: (e: { data: string }) => void
  constructor() { InertWebSocket.sockets.push(this) }
  close() { this.readyState = 3 }
}
;(globalThis as any).WebSocket = InertWebSocket
window.addEventListener('error', e => errors.push(e.message))
window.addEventListener('unhandledrejection', e => errors.push(String(e.reason)))
const pause = () => new Promise(resolve => setTimeout(resolve, 20))
async function settle() { await pause(); await pause() }
async function wait(check: () => boolean, label: string) {
  for (let i = 0; i < 100; i++) { if (check()) return; await pause() }
  throw Error('Setup timeout: ' + label)
}
const body = () => document.getElementById('root')!.textContent || ''
const button = (title: string) => document.querySelector<HTMLButtonElement>(`button[title="${title}"]`)
function click(title: string) { const el = button(title); if (!el) throw Error('Missing control: ' + title); el.click() }
function clickText(text: string) {
  const el = [...document.querySelectorAll('button')].find(e => e.textContent === text)
  if (!el) throw Error('Missing button: ' + text)
  el.click()
}
const path = (id: number) => `/api/characters/${id}/wishlist`
function hold(id: number, method = 'GET', suffix = '') {
  const h: Hold = { path: path(id) + suffix, method }; holds.push(h); return h
}
async function release(h: Hold, value: any, status = 200) {
  await wait(() => !!h.resolve, 'held request ' + h.path)
  h.resolve!(response(value, status)); await settle()
}
const loading = () => !!document.querySelector('#root .animate-spin')
const getCount = (id: number) => events.filter(e => e.path === path(id) && e.method === 'GET').length
let root: Root | null = null
async function mount() {
  root = createRoot(document.getElementById('root')!)
  root.render(<MemoryRouter><WishlistPage /></MemoryRouter>)
  await wait(() => body().includes('ALPHA TARGET'), 'initial Alpha')
}
const results: any[] = []
let current: any
function check(ok: boolean, assertion: string, detail?: any) { current.assertions.push({ assertion, pass: ok, detail }) }
async function test(name: string, run: () => Promise<void>) {
  events = []; holds = []; lists = { 1: list(1), 2: list(2) }; localStorage.clear()
  current = { name, assertions: [] }; results.push(current)
  try { await mount(); await run() } catch (e) { current.setupError = String(e) }
  current.body = body(); current.events = [...events]
  root?.unmount(); root = null
  for (const h of holds) h.resolve?.(response({ entries: [], slot_layout: [] }))
  await settle()
  current.pass = !current.setupError && current.assertions.length > 0 && current.assertions.every((a: any) => a.pass)
}
async function switchToBeta() { click('FixtureBeta'); await wait(() => body().includes('BETA TARGET'), 'Beta rendered') }
async function remove() { click('Remove from wishlist'); await settle(); clickText('Remove'); await settle() }

async function run() {
  await test('late A success cannot replace B entries or layout', async () => {
    const a = hold(1); click('Refresh'); await wait(() => !!a.resolve, 'A refresh'); await switchToBeta()
    await release(a, list(1, 'STALE ALPHA'))
    check(body().includes('BETA TARGET') && !body().includes('STALE ALPHA'), 'B entries survive late A success')
    click('Collapse all sections'); await settle()
    const write = events.filter(e => e.method === 'PUT').at(-1)
    check(write?.path === path(2) + '/slot-layout' && JSON.stringify(write.body.layout) === JSON.stringify([{ slot_bucket: 'Feet', position: 0, collapsed: true }]), 'current B collapse serializes only B layout', write)
  })
  await test('stale A error/finally cannot finish pending B', async () => {
    const a = hold(1); click('Refresh'); await wait(() => !!a.resolve, 'A refresh')
    const b = hold(2); click('FixtureBeta'); await wait(() => !!b.resolve, 'B pending')
    check(!body().includes('ALPHA TARGET') && !button('Collapse all sections'), 'prior entries and layout controls absent while B pending')
    await release(a, { error: 'STALE A ERROR' }, 500)
    check(loading() && !body().includes('STALE A ERROR') && !body().includes('ALPHA TARGET'), 'A catch/finally cannot publish into B')
    await release(b, list(2)); check(body().includes('BETA TARGET') && !loading(), 'B completes normally')
  })
  await test('A to B to A creates a fresh owner', async () => {
    const old = hold(1); click('Refresh'); await wait(() => !!old.resolve, 'old A')
    await switchToBeta(); lists[1] = list(1, 'FRESH ALPHA'); click('FixtureAlpha')
    await wait(() => body().includes('FRESH ALPHA'), 'fresh A')
    await release(old, list(1, 'STALE ALPHA'))
    check(body().includes('FRESH ALPHA') && !body().includes('STALE ALPHA'), 'old A generation cannot replace new A')
  })
  await test('same-owner latest refresh wins success error and finally', async () => {
    const first = hold(1); click('Refresh'); await wait(() => !!first.resolve, 'first')
    const second = hold(1); click('Refresh'); await wait(() => !!second.resolve, 'second')
    await release(first, { error: 'STALE REFRESH ERROR' }, 500)
    check(loading() && !body().includes('STALE REFRESH ERROR'), 'older refresh cannot publish error or finish newer pending load')
    await release(second, list(1, 'NEWEST ALPHA'))
    const third = hold(1); click('Refresh'); await wait(() => !!third.resolve, 'third')
    const fourth = hold(1); click('Refresh'); await wait(() => !!fourth.resolve, 'fourth')
    await release(fourth, list(1, 'LATEST ALPHA')); await release(third, list(1, 'OLDER ALPHA'))
    check(body().includes('LATEST ALPHA') && !body().includes('OLDER ALPHA'), 'older refresh success cannot replace latest')
    const failure = hold(1); click('Refresh'); await wait(() => !!failure.resolve, 'current error')
    await release(failure, { error: 'CURRENT ERROR' }, 500)
    check(body().includes('CURRENT ERROR') && !loading(), 'current refresh error is visible and finishes loading')
    click('Refresh'); await wait(() => body().includes('ALPHA TARGET') && !body().includes('CURRENT ERROR'), 'current recovery')
    check(!loading(), 'current refresh recovers after error')
  })
  await test('retired mutation success and failure cannot reload old owner', async () => {
    const deletion = hold(1, 'DELETE', '/1'); await remove(); await wait(() => !!deletion.resolve, 'delete')
    const layout = hold(1, 'PUT', '/slot-layout'); click('Collapse all sections'); await wait(() => !!layout.resolve, 'layout')
    await switchToBeta(); const before = getCount(1)
    await release(deletion, {}); await release(layout, { error: 'STALE MUTATION ERROR' }, 500)
    check(getCount(1) === before, 'retired delete success and layout failure issue no A reload', { before, after: getCount(1) })
    check(body().includes('BETA TARGET') && !body().includes('STALE MUTATION ERROR'), 'retired mutations cannot affect B rendering')
  })
  await test('old delete and search dialogs do not cross character switch', async () => {
    click('Remove from wishlist'); await settle(); check(body().includes('from the'), 'A delete confirmation opened')
    await switchToBeta()
    check(!body().includes('from the') && ![...document.querySelectorAll('button')].some(e => e.textContent === 'Remove'), 'A delete selection is invisible and unactionable on B')
    // Close only if the broken baseline retained it, so the second crossing also executes.
    if (body().includes('from the')) clickText('Cancel')
    click('Add item to wishlist'); await settle(); check(!!document.querySelector('input'), 'B search opened')
    click('FixtureAlpha'); await wait(() => body().includes('ALPHA TARGET'), 'Alpha again')
    check(!document.querySelector('input'), 'search dialog does not survive owner switch')
    check(!events.some(e => e.method === 'DELETE'), 'switching dialogs performs no delete')
  })
  await test('current-owner controls API payloads and websocket refresh remain usable', async () => {
    click('Flat list of all items'); await settle(); check(!!document.querySelector('[title="Slot: Head"]'), 'flat list uses current entries')
    click('Group by slot'); await settle()
    click('Auto-removed once looted — click to keep on wishlist instead (farm target)'); await settle()
    check(events.some(e => e.method === 'PATCH' && e.path === path(1) + '/1' && e.body.keep_after_loot === true), 'keep toggle serializes current owner')
    click('Add item to wishlist'); await settle()
    const input = document.querySelector('input')!
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, 'ADDED')
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await wait(() => body().includes('ADDED TARGET'), 'search result');
    const result = [...document.querySelectorAll('button')].find(e => e.textContent?.includes('ADDED TARGET'))!; result.click()
    await wait(() => !!button('ADDED TARGET'), 'added entry')
    check(events.some(e => e.method === 'POST' && e.path === path(1) && e.body.item_id === 33), 'add uses actual API')
    await remove(); await wait(() => !button('ALPHA TARGET'), 'deleted entry')
    check(events.some(e => e.method === 'DELETE' && e.path === path(1) + '/1'), 'current delete reloads normally')
    const before = getCount(1)
    const socket = InertWebSocket.sockets.at(-1)!
    socket.onmessage?.({ data: JSON.stringify({ type: 'wishlist:changed', data: { character_id: 2 } }) }); await settle()
    check(getCount(1) === before, 'other-character websocket event ignored')
    lists[1] = list(1, 'WS REFRESHED')
    socket.onmessage?.({ data: JSON.stringify({ type: 'wishlist:changed', data: { character_id: 1 } }) })
    await wait(() => body().includes('WS REFRESHED'), 'websocket refresh'); check(getCount(1) === before + 1, 'current websocket event refreshes once')
  })
  await test('unmount retires pending load and mutation across remount', async () => {
    const mutation = hold(1, 'DELETE', '/1'); await remove(); await wait(() => !!mutation.resolve, 'mutation')
    const load = hold(1); click('Refresh'); await wait(() => !!load.resolve, 'load')
    root!.unmount(); root = null; const before = getCount(1)
    await release(mutation, {}); await release(load, list(1, 'UNMOUNTED ALPHA'))
    check(getCount(1) === before && body() === '', 'unmounted continuation sends no reload and renders nothing')
    await mount(); check(body().includes('ALPHA TARGET') && !body().includes('UNMOUNTED ALPHA'), 'remount owns fresh data')
  })
  document.getElementById('result')!.textContent = JSON.stringify({ status: results.every(r => r.pass) && !errors.length ? 'PASS' : 'FAIL', cases: results, errors }, null, 2)
}
run().catch(e => { document.getElementById('result')!.textContent = JSON.stringify({ status: 'HARNESS_FAILURE', error: String(e), results, errors }) })
