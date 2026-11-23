// Floor lookup for instanced 3D-map objects. A zone's trees, bridges and
// building floors are drawn as instances of a few models; standing a
// character model on them needs the floor heights under a point, which the
// zone's own triangle grid can't give without copying every instance's
// triangles back out. This grid holds instance references instead: each
// instance is filed under the cells its floor footprint covers, and a query
// places only the nearby instances' floor triangles.

// InstancedFloorModel is one model's floor triangles and its placements.
// pos is the model's vertices (x, y, z each) in the same space the matrices
// map from; tris indexes pos, three per floor triangle; mats holds a 3x4
// row-major matrix per instance (12 numbers each) into world space.
export interface InstancedFloorModel {
  pos: ArrayLike<number>
  tris: ArrayLike<number>
  mats: ArrayLike<number>
}

export interface InstanceFloorGrid {
  models: InstancedFloorModel[]
  cell: number
  // cells maps a cell key to instance references, model * REF_SPLIT +
  // instance.
  cells: Map<number, number[]>
}

const REF_SPLIT = 1 << 20

function cellKey(cx: number, cy: number): number {
  return (cx + 32768) * 65536 + (cy + 32768)
}

// buildInstanceFloorGrid files every instance of every model under the
// cells its floor triangles' world bounding box covers.
export function buildInstanceFloorGrid(models: InstancedFloorModel[], cell: number): InstanceFloorGrid {
  const cells = new Map<number, number[]>()
  models.forEach((m, mi) => {
    if (m.tris.length === 0) return
    // The floor triangles' model-space box; its 8 corners bound any
    // affine placement of them.
    const lo = [Infinity, Infinity, Infinity]
    const hi = [-Infinity, -Infinity, -Infinity]
    for (let i = 0; i < m.tris.length; i++) {
      const v = m.tris[i] * 3
      for (let k = 0; k < 3; k++) {
        lo[k] = Math.min(lo[k], m.pos[v + k])
        hi[k] = Math.max(hi[k], m.pos[v + k])
      }
    }
    const n = m.mats.length / 12
    for (let j = 0; j < n; j++) {
      const o = j * 12
      let x0 = Infinity, x1 = -Infinity, y0 = Infinity, y1 = -Infinity
      for (let c = 0; c < 8; c++) {
        const px = c & 1 ? hi[0] : lo[0]
        const py = c & 2 ? hi[1] : lo[1]
        const pz = c & 4 ? hi[2] : lo[2]
        const wx = m.mats[o] * px + m.mats[o + 1] * py + m.mats[o + 2] * pz + m.mats[o + 3]
        const wy = m.mats[o + 4] * px + m.mats[o + 5] * py + m.mats[o + 6] * pz + m.mats[o + 7]
        x0 = Math.min(x0, wx); x1 = Math.max(x1, wx)
        y0 = Math.min(y0, wy); y1 = Math.max(y1, wy)
      }
      const ref = mi * REF_SPLIT + j
      for (let cx = Math.floor(x0 / cell); cx <= Math.floor(x1 / cell); cx++) {
        for (let cy = Math.floor(y0 / cell); cy <= Math.floor(y1 / cell); cy++) {
          const k = cellKey(cx, cy)
          const list = cells.get(k)
          if (list) list.push(ref)
          else cells.set(k, [ref])
        }
      }
    }
  })
  return { models, cell, cells }
}

// instanceFloorsAt calls visit with the world height of every instanced
// floor face over world point (x, y), testing faces exactly as the zone's
// own triangle grid does.
export function instanceFloorsAt(
  grid: InstanceFloorGrid, x: number, y: number, visit: (z: number) => void,
): void {
  const list = grid.cells.get(cellKey(Math.floor(x / grid.cell), Math.floor(y / grid.cell)))
  if (!list) return
  const w = [0, 0, 0, 0, 0, 0, 0, 0, 0]
  for (const ref of list) {
    const m = grid.models[Math.floor(ref / REF_SPLIT)]
    const o = (ref % REF_SPLIT) * 12
    const t = m.mats
    for (let i = 0; i + 2 < m.tris.length; i += 3) {
      for (let k = 0; k < 3; k++) {
        const v = m.tris[i + k] * 3
        const px = m.pos[v], py = m.pos[v + 1], pz = m.pos[v + 2]
        w[k * 3] = t[o] * px + t[o + 1] * py + t[o + 2] * pz + t[o + 3]
        w[k * 3 + 1] = t[o + 4] * px + t[o + 5] * py + t[o + 6] * pz + t[o + 7]
        w[k * 3 + 2] = t[o + 8] * px + t[o + 9] * py + t[o + 10] * pz + t[o + 11]
      }
      const x1 = w[0], y1 = w[1], x2 = w[3], y2 = w[4], x3 = w[6], y3 = w[7]
      const d = (y2 - y3) * (x1 - x3) + (x3 - x2) * (y1 - y3)
      if (d === 0) continue
      const l1 = ((y2 - y3) * (x - x3) + (x3 - x2) * (y - y3)) / d
      const l2 = ((y3 - y1) * (x - x3) + (x1 - x3) * (y - y3)) / d
      const l3 = 1 - l1 - l2
      if (l1 < -1e-4 || l2 < -1e-4 || l3 < -1e-4) continue
      visit(l1 * w[2] + l2 * w[5] + l3 * w[8])
    }
  }
}
