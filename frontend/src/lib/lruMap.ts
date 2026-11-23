// LruMap is a size-capped Map for the module-level session caches (items,
// spells). Those caches live as long as the window, and the NPC / item info
// overlays look up a steady stream of new ids over a raid, so an uncapped Map
// only ever grows. Reads refresh an entry's recency; inserting past the cap
// drops the least recently used entry. An evicted id is simply fetched again
// the next time it's needed.
export class LruMap<K, V> {
  private map = new Map<K, V>()
  private readonly max: number

  constructor(max: number) {
    this.max = max
  }

  get(key: K): V | undefined {
    const v = this.map.get(key)
    if (v === undefined) return undefined
    // Map keeps insertion order: re-inserting moves the key to the newest end.
    this.map.delete(key)
    this.map.set(key, v)
    return v
  }

  set(key: K, value: V): void {
    this.map.delete(key)
    this.map.set(key, value)
    if (this.map.size > this.max) {
      const oldest = this.map.keys().next()
      if (!oldest.done) this.map.delete(oldest.value)
    }
  }

  get size(): number {
    return this.map.size
  }
}
