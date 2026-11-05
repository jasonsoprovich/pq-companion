import { computeAnchorMs, acceptNewAnchor } from '../src/lib/chMetronome'
const cases: any[] = []
const base = 1800000000000
function timer(slot: number, at: number, now: number, target = 'Tank', category = 'ch_chain'): any {
  return { id: `${target}:${slot}`, spell_name: `#${slot} ${target} ← ${slot === 1 ? 'You' : 'Cleric'}`, target_name: target, category,
    starts_at: new Date(base + at * 1000).toISOString(), remaining_seconds: 10 - (now - at), duration_seconds: 10 }
}
function scenario(enabled: boolean | undefined, coded = false, mode = 'drop', size = 5) {
  localStorage.clear()
  const seen = new Map<number, number>(), cfg = { position: 1, chainSize: size, delay: 4, sustainedDropout: enabled }
  let rows: any[] = [], accepted: any = null, last: any
  const trace: any[] = []
  const send = (slot: number, at: number, extra: any[] = []) => {
    rows = rows.filter(r => at - r.at < 10 && r.slot !== slot)
    rows.push({ slot, at })
    last = computeAnchorMs([...rows.map(r => timer(coded ? r.slot * 111 : r.slot, r.at, at)), ...extra], cfg, 'main', seen, base + at * 1000)
    if (last && acceptNewAnchor(accepted, last, cfg)) accepted = last
    trace.push({ at, last, accepted })
    return last
  }
  if (mode !== 'unknown') for (let at = 0; at < size * 12; at += 4) send(at / 4 % size + 1, at)
  for (let at = size * 12; at <= 196; at += 4) {
    if (mode === 'once' && at === 76) continue
    const ring = mode === 'continuous' || mode === 'once' ? size : mode === 'multi' ? 2 : size - 1
    send((at - size * 12) / 4 % ring + 1, at,
      mode === 'concurrent' ? [timer(99, at, at, 'Other'), timer(99, at, at, 'Ramp', 'ch_chain_2')] : [])
    if (at === 136 && mode === 'drop') break
  }
  return { get last() { return last }, get accepted() { return accepted }, send, seen, cfg, trace }
}
try {
  const off = scenario(false), omitted = scenario(undefined), on = scenario(true), coded = scenario(true, true)
  cases.push({ name: 'default-off-preserved', pass: JSON.stringify(off.last) === JSON.stringify(omitted.last), assertions: ['missing equals false'], observed: off.trace })
  cases.push({ name: 'sustained-dropout-actual-compute-accept', pass: on.last?.anchorMs === base + 136000 && on.accepted?.anchorMs === base + 136000, assertions: ['watch #4 at 136, not projected #5 at 140'], observed: on })
  cases.push({ name: 'coded-roster-stays-ranked', pass: coded.last?.anchorMs === base + 136000, assertions: ['555 remains slot 5 after >60s absence'], observed: coded.last })
  const record = (name: string, pass: boolean, observed: unknown, assertion: string) => cases.push({ name, pass, observed, assertions: [assertion] })
  const rejoin = on.send(5, 140) as any
  record('immediate-rejoin', rejoin?.watch === 5 && rejoin?.skipped?.length === 0 && on.accepted.anchorMs === base + 140000, rejoin, 'genuine #5 reinstates normal watch through accept')
  for (const mode of ['continuous', 'once', 'unknown']) {
    const control = scenario(true, false, mode)
    record(mode, control.trace.every(v => !v.last?.skipped?.length), control.trace, 'complete/single-miss/unknown roster cannot remove slot')
  }
  const full = scenario(true, false, 'continuous'), legacyFull = scenario(false, false, 'continuous')
  record('continuous-timing-parity', full.trace.every((v, i) => v.accepted?.anchorMs === legacyFull.trace[i].accepted?.anchorMs), { adaptive: full.trace, legacy: legacyFull.trace }, 'continuous accepted anchors match legacy')
  const stale = scenario(true)
  const staleResult = computeAnchorMs([timer(4, 136, 136), { ...timer(5, 56, 136), possible_miss: true }], stale.cfg, 'main', stale.seen, base + 200000)
  record('stale-frozen-and-expired-feed', staleResult === null, staleResult, 'identical starts cannot refresh absence evidence')
  const fresh = stale.send(1, 204) as any
  record('idle-retirement', !fresh?.skipped?.length, fresh, 'idle chain relearns from scratch')
  const scoped = scenario(true)
  const changed = computeAnchorMs([timer(1, 140, 140, 'Other')], scoped.cfg, 'main', scoped.seen, base + 140000) as any
  record('target-change', !changed?.skipped?.length && scoped.seen.size === 1, changed, 'target change retires former roster')
  const ramp = computeAnchorMs([timer(1, 144, 144, 'Ramp', 'ch_chain_2')], scoped.cfg, 'ramp', scoped.seen, base + 144000) as any
  record('main-ramp-separation', !ramp?.skipped?.length && scoped.seen.size === 1, ramp, 'new category cannot inherit evidence')
  const concurrent = scenario(true, false, 'concurrent')
  record('concurrent-target-category', concurrent.last?.watch === 4 && concurrent.last?.skipped?.join() === '5', concurrent.last, 'other tank/category excluded before learning')
  const multi = scenario(true, false, 'multi', 6)
  record('multiple-dropout-cycle', multi.last?.effectiveChainSize === 2 && multi.last?.watch === 2 && multi.last?.skipped?.join() === '3,4,5,6', multi.last, 'six-slot rotation compresses to two')
  const reload = computeAnchorMs([timer(4, 136, 136)], on.cfg, 'main', new Map(on.seen), base + 136000) as any
  record('reload-relearns', !reload?.skipped?.length, reload, 'persisted learned map cannot establish dropout')
  const disabled = scenario(true); disabled.cfg.sustainedDropout = false
  const disableResult = computeAnchorMs([timer(4, 140, 140)], disabled.cfg, 'main', disabled.seen, base + 140000)
  record('opt-out-math', disableResult?.anchorMs === base + 144000, disableResult, 'off restores uncompressed projection')
} catch (error) { cases.push({ name: 'exception', pass: false, error: String(error), assertions: [] }) }
document.getElementById('result')!.textContent = JSON.stringify({ status: cases.every(c => c.pass) ? 'PASS' : 'FAIL', cases })
