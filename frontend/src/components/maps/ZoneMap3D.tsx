import React, { useEffect, useRef, useState } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import { RotateCcw, SlidersHorizontal } from 'lucide-react'
import { getZoneMesh3D, getZoneTextureUrl } from '../../services/api'
import type { ZoneMesh3D } from '../../services/api'
import { useMap3DPrefs } from '../../hooks/useMap3DPrefs'
import type { Map3DPrefs, Map3DStyle } from '../../hooks/useMap3DPrefs'
import type { RaiderMarker, ReplayTarget } from './ZoneMap'

// ZoneMap3D draws a zone in 3D from the user's own installed client files
// (<eq_path>/<zone>.s3d, read by the backend): the zone's geometry with its
// own textures, or coloured by height or plainly, with markers (raiders, or
// you and your group) and pins on top. Floors, walls and ceilings are drawn
// as separate layers so walls can be faded and ceilings hidden. Loaded
// lazily, so three.js only downloads when someone opens the 3D view.
//
// World axes: map space x, -y, z, with z up. Map space y grows downward on
// the 2D map, so negating it keeps north at the top of the default view and
// left/right the same as the 2D map.

// Pin3D is a fixed point drawn as a small diamond: a map pin, a user marker,
// or a kill mark in a replay. In map space, like markers.
export interface Pin3D {
  x: number
  y: number
  z: number
  color: string
  label: string
}

export interface ZoneMap3DProps {
  zone: string
  // markers are raiders in a replay, or you and your group live. The camera
  // follows the one with focus set.
  markers: RaiderMarker[]
  // target draws a line from the observer to their target.
  target?: ReplayTarget | null
  pins?: Pin3D[]
  // onUnavailable fires when 3D can't be shown (no WebGL, no s3d), so the
  // caller can fall back to 2D.
  onUnavailable?: (reason: string) => void
}

// HEIGHT_RAMP colours the height style, low to high: deep blue through
// teal and green to sand, so a tall zone reads as more than one colour.
const HEIGHT_RAMP = ['#1d3557', '#2a6f97', '#3a9d8f', '#8fbf6a', '#e9d8a6'].map((c) => new THREE.Color(c))

// Faces steeper than this (|normal z|) count as floors or ceilings; the rest
// are walls. Matches the backend's walkable-floor threshold.
const FLAT = 0.55

// Names show once a marker is drawn at least LABEL_PX pixels across, and a
// pin PIN_LABEL_PX: a whole raid's names from far away is a smear.
const LABEL_PX = 18
const PIN_LABEL_PX = 13

function heightColors(pos: Float32Array): Float32Array {
  let lo = Infinity
  let hi = -Infinity
  for (let i = 2; i < pos.length; i += 3) {
    lo = Math.min(lo, pos[i])
    hi = Math.max(hi, pos[i])
  }
  const span = hi > lo ? hi - lo : 1
  const out = new Float32Array(pos.length)
  const c = new THREE.Color()
  const last = HEIGHT_RAMP.length - 1
  for (let i = 0; i < pos.length; i += 3) {
    const t = ((pos[i + 2] - lo) / span) * last
    const k = Math.min(last - 1, Math.floor(t))
    c.lerpColors(HEIGHT_RAMP[k], HEIGHT_RAMP[k + 1], t - k)
    out[i] = c.r
    out[i + 1] = c.g
    out[i + 2] = c.b
  }
  return out
}

// Layers is one set of geometry split by which way each face points.
interface Layers {
  floor: THREE.Mesh
  wall: THREE.Mesh
  ceiling: THREE.Mesh
}

// ZoneLayers is the zone's own geometry and its placed objects (trees,
// buildings, furniture), each split into layers. All six meshes share one
// set of vertex attributes; each has a draw group per zone material, so
// materials can be swapped without rebuilding them.
interface ZoneLayers {
  zone: string
  data: ZoneMesh3D
  own: Layers
  objects: Layers
  // group holds all six meshes.
  group: THREE.Group
  // textures by file name (and whether masked), loaded the first time the
  // textured style is shown.
  textures: Map<string, THREE.Texture>
  // materials currently assigned, freed when the look changes; faded is
  // the walls' and ceilings' array, whose opacity follows the slider.
  materials: THREE.Material[]
  faded: THREE.Material[]
}

interface Scene3D {
  renderer: THREE.WebGLRenderer
  scene: THREE.Scene
  camera: THREE.PerspectiveCamera
  controls: OrbitControls
  markers: THREE.Group
  pins: THREE.Group
  targetLine: THREE.Line
  layers: ZoneLayers | null
  markerSize: number
  // home is the starting camera, for Reset view.
  home: { position: THREE.Vector3; target: THREE.Vector3 } | null
  render: () => void
  dispose: () => void
}

export default function ZoneMap3D({ zone, markers, target, pins, onUnavailable }: ZoneMap3DProps): React.ReactElement {
  const host = useRef<HTMLDivElement>(null)
  const labelCanvas = useRef<HTMLCanvasElement>(null)
  const scene = useRef<Scene3D | null>(null)
  const markersRef = useRef(markers)
  markersRef.current = markers
  const pinsRef = useRef(pins)
  pinsRef.current = pins
  const [status, setStatus] = useState<'loading' | 'ready' | string>('loading')
  const lastFocus = useRef<THREE.Vector3 | null>(null)
  const [prefs, setPrefs] = useMap3DPrefs()
  const prefsRef = useRef(prefs)
  prefsRef.current = prefs
  const [showOptions, setShowOptions] = useState(false)

  // Build the scene once per zone.
  useEffect(() => {
    const el = host.current
    if (!el) return
    let cancelled = false
    let renderer: THREE.WebGLRenderer
    try {
      renderer = new THREE.WebGLRenderer({ antialias: true })
    } catch {
      setStatus('3D needs WebGL, which isn’t available here.')
      onUnavailable?.('webgl')
      return
    }
    renderer.setPixelRatio(window.devicePixelRatio)
    renderer.setClearColor(0x0b0d12)
    el.appendChild(renderer.domElement)
    renderer.domElement.style.display = 'block'

    const sc = new THREE.Scene()
    const camera = new THREE.PerspectiveCamera(50, 1, 1, 100000)
    camera.up.set(0, 0, 1)
    const controls = new OrbitControls(camera, renderer.domElement)
    controls.screenSpacePanning = true
    sc.add(new THREE.HemisphereLight(0xdde6f5, 0x202430, 1.6))
    const sun = new THREE.DirectionalLight(0xffffff, 1.4)
    sun.position.set(0.4, -0.6, 1)
    sc.add(sun)
    const markerGroup = new THREE.Group()
    markerGroup.renderOrder = 10
    sc.add(markerGroup)
    const pinGroup = new THREE.Group()
    pinGroup.renderOrder = 8
    sc.add(pinGroup)
    const targetLine = new THREE.Line(
      new THREE.BufferGeometry(),
      new THREE.LineDashedMaterial({ color: 0xf87171, dashSize: 6, gapSize: 4, depthTest: false, transparent: true }),
    )
    targetLine.renderOrder = 9
    targetLine.visible = false
    sc.add(targetLine)

    let frame = 0
    const s: Scene3D = {
      renderer, scene: sc, camera, controls, markers: markerGroup, pins: pinGroup, targetLine,
      layers: null, markerSize: 4, home: null,
      render: () => {
        if (frame) return
        frame = requestAnimationFrame(() => {
          frame = 0
          const w = el.clientWidth
          const h = el.clientHeight
          if (w > 0 && h > 0) {
            const size = renderer.getSize(new THREE.Vector2())
            if (size.x !== w || size.y !== h) {
              renderer.setSize(w, h, false)
              renderer.domElement.style.width = '100%'
              renderer.domElement.style.height = '100%'
              camera.aspect = w / h
              camera.updateProjectionMatrix()
            }
          }
          renderer.render(sc, camera)
          drawLabels(s, labelCanvas.current, w, h, markersRef.current, pinsRef.current ?? [])
        })
      },
      dispose: () => {
        cancelAnimationFrame(frame)
        ro.disconnect()
        controls.dispose()
        // Marker and pin geometry and materials are shared module-wide; only
        // the zone's own meshes, materials and textures and the target line
        // are freed.
        sc.remove(markerGroup)
        sc.remove(pinGroup)
        if (s.layers) {
          s.layers.materials.forEach((m) => m.dispose())
          s.layers.textures.forEach((t) => t.dispose())
          for (const m of layerMeshes(s.layers)) m.geometry.dispose()
          sc.remove(s.layers.group)
          s.layers = null
        }
        sc.traverse((o) => {
          const m = o as THREE.Mesh
          m.geometry?.dispose()
          const mat = m.material as THREE.Material | THREE.Material[] | undefined
          if (Array.isArray(mat)) mat.forEach((x) => x.dispose())
          else mat?.dispose()
        })
        renderer.dispose()
        renderer.domElement.remove()
      },
    }
    controls.addEventListener('change', s.render)
    const ro = new ResizeObserver(s.render)
    ro.observe(el)
    scene.current = s
    setStatus('loading')

    getZoneMesh3D(zone)
      .then((mesh) => {
        if (cancelled) return
        if (!mesh || mesh.indices.length === 0) {
          setStatus(`No ${zone}.s3d in your EverQuest folder, so there’s no 3D for this zone.`)
          onUnavailable?.('no-mesh')
          return
        }
        const layers = buildLayers(zone, mesh)
        s.layers = layers
        sc.add(layers.group)
        buildMaterials(s, prefsRef.current)
        adjustLook(layers, prefsRef.current)

        const box = new THREE.Box3()
          .setFromBufferAttribute(layers.own.floor.geometry.getAttribute('position') as THREE.BufferAttribute)
        const center = box.getCenter(new THREE.Vector3())
        const extent = box.getSize(new THREE.Vector3())
        const radius = Math.max(extent.x, extent.y, 1)
        s.markerSize = Math.max(3, radius / 250)
        // Start over the markers if there are any, else over the zone,
        // looking north and down at about 50 degrees.
        const start = markerCentroid(markersRef.current) ?? center
        const dist = markersRef.current.length ? radius * 0.35 : radius * 0.9
        camera.near = Math.max(1, radius / 5000)
        camera.far = radius * 10
        camera.updateProjectionMatrix()
        s.home = {
          target: start.clone(),
          position: new THREE.Vector3(start.x, start.y - dist * 0.65, start.z + dist * 0.75),
        }
        resetView(s)
        setStatus('ready')
        updateMarkers(s, markersRef.current)
        updatePins(s, pinsRef.current ?? [])
        s.render()
      })
      .catch((err: Error) => {
        if (cancelled) return
        setStatus(`Couldn’t load the 3D zone: ${err.message}`)
        onUnavailable?.('error')
      })

    return () => {
      cancelled = true
      scene.current = null
      s.dispose()
    }
    // onUnavailable is a notification; rebuilding the scene for a new
    // callback identity would reload the mesh.
  }, [zone])

  // Markers follow the playhead or the live feed; the camera follows the
  // focused one.
  useEffect(() => {
    const s = scene.current
    if (!s || status !== 'ready') return
    updateMarkers(s, markers)
    updateTargetLine(s, target ?? null)
    const f = markers.find((r) => r.focus)
    if (f) {
      const at = new THREE.Vector3(f.x, -f.y, f.z)
      if (lastFocus.current) {
        const d = at.clone().sub(lastFocus.current)
        s.camera.position.add(d)
        s.controls.target.add(d)
      } else {
        const d = at.clone().sub(s.controls.target)
        s.camera.position.add(d)
        s.controls.target.copy(at)
      }
      lastFocus.current = at
      s.controls.update()
    } else {
      lastFocus.current = null
    }
    s.render()
  }, [markers, target, status])

  useEffect(() => {
    const s = scene.current
    if (!s || status !== 'ready') return
    updatePins(s, pins ?? [])
    s.render()
  }, [pins, status])

  // A new style or culling needs new materials; opacity and what's shown
  // only adjust the ones there, so dragging the slider stays cheap.
  useEffect(() => {
    const s = scene.current
    if (!s || status !== 'ready' || !s.layers) return
    buildMaterials(s, prefsRef.current)
    adjustLook(s.layers, prefsRef.current)
    s.render()
  }, [prefs.style, prefs.seeThroughOuter, status])

  useEffect(() => {
    const s = scene.current
    if (!s || status !== 'ready' || !s.layers) return
    adjustLook(s.layers, prefs)
    s.render()
  }, [prefs.wallOpacity, prefs.hideCeilings, prefs.showObjects, status])

  return (
    <div className="relative h-full w-full overflow-hidden rounded" style={{ backgroundColor: '#0b0d12' }}>
      <div ref={host} className="absolute inset-0" />
      <canvas ref={labelCanvas} className="pointer-events-none absolute inset-0 h-full w-full" />
      {status !== 'ready' && (
        <div className="absolute inset-0 flex items-center justify-center p-6 text-center text-sm" style={{ color: 'var(--color-muted)' }}>
          {status === 'loading' ? 'Loading 3D zone…' : status}
        </div>
      )}
      {status === 'ready' && (
        <>
          <div className="absolute right-2 top-2 flex items-center gap-1">
            <button
              className="flex items-center gap-1 rounded border px-1.5 py-0.5 text-[10px]"
              style={{
                backgroundColor: showOptions ? 'var(--color-surface-2)' : 'rgba(11,13,18,0.8)',
                borderColor: 'var(--color-border)',
                color: 'var(--color-muted-foreground)',
              }}
              title="Textures, wall transparency and ceilings"
              onClick={() => setShowOptions((v) => !v)}
            >
              <SlidersHorizontal size={10} /> View
            </button>
            <button
              className="flex items-center gap-1 rounded border px-1.5 py-0.5 text-[10px]"
              style={{ backgroundColor: 'rgba(11,13,18,0.8)', borderColor: 'var(--color-border)', color: 'var(--color-muted-foreground)' }}
              title="Back to the starting view"
              onClick={() => {
                const s = scene.current
                if (!s) return
                lastFocus.current = null
                resetView(s)
                s.render()
              }}
            >
              <RotateCcw size={10} /> Reset view
            </button>
          </div>
          {showOptions && <ViewOptions prefs={prefs} onChange={setPrefs} />}
          <div className="pointer-events-none absolute bottom-2 left-2 text-[10px]" style={{ color: 'rgba(255,255,255,0.45)' }}>
            Drag to orbit · right-drag to pan · scroll to zoom
          </div>
        </>
      )}
    </div>
  )
}

// ViewOptions is the 3D view's look: style, wall transparency, and what to
// hide so the inside of a zone can be seen from outside.
function ViewOptions({ prefs, onChange }: {
  prefs: Map3DPrefs
  onChange: (patch: Partial<Map3DPrefs>) => void
}): React.ReactElement {
  const styles: [Map3DStyle, string, string][] = [
    ['textured', 'Textured', 'The zone’s own textures, from your EverQuest files'],
    ['height', 'Height', 'Coloured from low (blue) to high (sand)'],
    ['plain', 'Plain', 'Light floors, dark walls'],
  ]
  return (
    <div
      className="absolute right-2 top-8 w-52 space-y-2.5 rounded border p-2.5 text-[11px]"
      style={{ backgroundColor: 'rgba(11,13,18,0.92)', borderColor: 'var(--color-border)', color: 'var(--color-foreground)' }}
    >
      <div>
        <div className="mb-1" style={{ color: 'var(--color-muted)' }}>Style</div>
        <div className="flex overflow-hidden rounded border" style={{ borderColor: 'var(--color-border)' }}>
          {styles.map(([v, label, title]) => (
            <button
              key={v}
              className="flex-1 px-1 py-0.5"
              title={title}
              style={{
                backgroundColor: prefs.style === v ? 'var(--color-primary)' : 'transparent',
                color: prefs.style === v ? 'var(--color-background)' : 'var(--color-muted-foreground)',
              }}
              onClick={() => onChange({ style: v })}
            >
              {label}
            </button>
          ))}
        </div>
      </div>
      <label className="block">
        <div className="mb-1 flex justify-between" style={{ color: 'var(--color-muted)' }}>
          <span>Wall opacity</span>
          <span>{Math.round(prefs.wallOpacity * 100)}%</span>
        </div>
        <input
          type="range"
          min={10}
          max={100}
          step={5}
          value={Math.round(prefs.wallOpacity * 100)}
          onChange={(e) => onChange({ wallOpacity: Number(e.target.value) / 100 })}
          className="w-full"
        />
      </label>
      <label className="flex items-start gap-1.5" title="Hide the back of every wall, so from outside you look in instead of at a solid box">
        <input
          type="checkbox"
          className="mt-0.5"
          checked={prefs.seeThroughOuter}
          onChange={(e) => onChange({ seeThroughOuter: e.target.checked })}
        />
        <span>See in through outer walls</span>
      </label>
      <label className="flex items-start gap-1.5">
        <input
          type="checkbox"
          className="mt-0.5"
          checked={prefs.showObjects}
          onChange={(e) => onChange({ showObjects: e.target.checked })}
        />
        <span>Trees &amp; objects</span>
      </label>
      <label className="flex items-start gap-1.5">
        <input
          type="checkbox"
          className="mt-0.5"
          checked={prefs.hideCeilings}
          onChange={(e) => onChange({ hideCeilings: e.target.checked })}
        />
        <span>Hide ceilings</span>
      </label>
    </div>
  )
}

// buildLayers turns the zone mesh into floor, wall and ceiling meshes, for
// the zone's own geometry and for its objects. World y is map y negated
// (see the header comment).
function buildLayers(zone: string, data: ZoneMesh3D): ZoneLayers {
  const pos = new Float32Array(data.positions)
  for (let i = 1; i < pos.length; i += 3) pos[i] = -pos[i]
  const position = new THREE.BufferAttribute(pos, 3)
  const uv = new THREE.BufferAttribute(data.uvs, 2)
  const color = new THREE.BufferAttribute(heightColors(pos), 3)

  // Split each material's triangles three ways by facing, keeping a draw
  // group per material in each layer: parts 0-2 are the zone's floors,
  // walls and ceilings, 3-5 its objects'.
  type Part = { idx: number[]; groups: { start: number; count: number; material: number }[] }
  const parts: Part[] = [0, 1, 2, 3, 4, 5].map(() => ({ idx: [], groups: [] }))
  const ix = data.indices
  for (const g of data.groups) {
    const base = data.materials[g.material].object ? 3 : 0
    const starts = parts.map((p) => p.idx.length)
    for (let i = g.start; i < g.start + g.count; i += 3) {
      const a = ix[i] * 3
      const b = ix[i + 1] * 3
      const c = ix[i + 2] * 3
      const ux = pos[b] - pos[a], uy = pos[b + 1] - pos[a + 1], uz = pos[b + 2] - pos[a + 2]
      const vx = pos[c] - pos[a], vy = pos[c + 1] - pos[a + 1], vz = pos[c + 2] - pos[a + 2]
      const nx = uy * vz - uz * vy
      const ny = uz * vx - ux * vz
      const nz = ux * vy - uy * vx
      const len = Math.hypot(nx, ny, nz) || 1
      const up = nz / len
      const part = parts[base + (up >= FLAT ? 0 : up <= -FLAT ? 2 : 1)]
      part.idx.push(ix[i], ix[i + 1], ix[i + 2])
    }
    parts.forEach((p, k) => {
      const n = p.idx.length - starts[k]
      if (n > 0) p.groups.push({ start: starts[k], count: n, material: g.material })
    })
  }
  const meshes = parts.map((p) => {
    const geo = new THREE.BufferGeometry()
    geo.setAttribute('position', position)
    geo.setAttribute('uv', uv)
    geo.setAttribute('color', color)
    geo.setIndex(new THREE.BufferAttribute(new Uint32Array(p.idx), 1))
    for (const g of p.groups) geo.addGroup(g.start, g.count, g.material)
    geo.computeVertexNormals()
    return new THREE.Mesh(geo, [])
  })
  const group = new THREE.Group()
  group.add(...meshes)
  return {
    zone,
    data,
    own: { floor: meshes[0], wall: meshes[1], ceiling: meshes[2] },
    objects: { floor: meshes[3], wall: meshes[4], ceiling: meshes[5] },
    group,
    textures: new Map(),
    materials: [],
    faded: [],
  }
}

function layerMeshes(L: ZoneLayers): THREE.Mesh[] {
  return [L.own.floor, L.own.wall, L.own.ceiling, L.objects.floor, L.objects.wall, L.objects.ceiling]
}

// texture returns a zone material's texture, starting its load the first
// time; s.render runs as each one arrives.
function texture(s: Scene3D, L: ZoneLayers, i: number): THREE.Texture | null {
  const m = L.data.materials[i]
  if (!m.texture) return null
  const masked = m.kind === 1
  const key = masked ? `${m.texture}|mask` : m.texture
  let t = L.textures.get(key)
  if (!t) {
    const tex = new THREE.Texture()
    // Client UVs run top-down (DirectX), and repeat across large faces.
    tex.flipY = false
    tex.wrapS = THREE.RepeatWrapping
    tex.wrapT = THREE.RepeatWrapping
    tex.colorSpace = THREE.SRGBColorSpace
    tex.anisotropy = s.renderer.capabilities.getMaxAnisotropy()
    L.textures.set(key, tex)
    // Fetched rather than loaded as an <img>: the page's CSP allows the
    // backend for connections but not for images. An ImageBitmap is never
    // flipped on upload, which suits the top-down UVs.
    getZoneTextureUrl(L.zone, m.texture, masked)
      .then((url) => fetch(url))
      .then((res) => (res.ok ? res.blob() : Promise.reject(new Error(res.statusText))))
      .then((blob) => createImageBitmap(blob, { premultiplyAlpha: 'none', colorSpaceConversion: 'none' }))
      .then((bitmap) => {
        // The zone may have been closed while this loaded.
        if (s.layers !== L) {
          bitmap.close()
          return
        }
        tex.image = bitmap
        tex.needsUpdate = true
        s.render()
      })
      .catch(() => {
        // One the backend can't serve is drawn plain grey rather than black.
        if (s.layers !== L) return
        tex.image = new ImageData(new Uint8ClampedArray([138, 143, 152, 255]), 1, 1)
        tex.needsUpdate = true
        s.render()
      })
    t = tex
  }
  return t
}

// buildMaterials makes the zone's materials for the style and culling.
// Material indices match the zone's own. Floors get one array and walls and
// ceilings another, built opaque; adjustLook fades the second. Each
// material's userData keeps its own opacity and whether it blends anyway
// (water, glows), which fading starts from.
function buildMaterials(s: Scene3D, prefs: Map3DPrefs): void {
  const L = s.layers
  if (!L) return
  L.materials.forEach((m) => m.dispose())
  L.materials = []
  const side = prefs.seeThroughOuter ? THREE.FrontSide : THREE.DoubleSide
  const make = (i: number, faded: boolean): THREE.Material => {
    const z = L.data.materials[i]
    let m: THREE.Material
    if (prefs.style === 'textured') {
      const map = texture(s, L, i)
      if (z.kind === 3) {
        m = new THREE.MeshBasicMaterial({
          map, transparent: true, blending: THREE.AdditiveBlending, depthWrite: false, side: THREE.DoubleSide,
        })
      } else {
        m = new THREE.MeshLambertMaterial({
          map,
          color: map ? 0xffffff : 0x8a8f98,
          // Client textures are made for the game's own lighting and look
          // murky under the map's; a little self-light lifts them.
          emissive: map ? 0xffffff : 0x000000,
          emissiveMap: map,
          emissiveIntensity: 0.35,
          side: z.kind === 1 ? THREE.DoubleSide : side,
          alphaTest: z.kind === 1 ? 0.5 : 0,
          transparent: z.kind === 2,
          opacity: z.kind === 2 ? z.opacity : 1,
          depthWrite: z.kind !== 2,
        })
      }
    } else if (z.kind === 1 || z.kind === 3) {
      // Cutouts (trees, fences) and glows are texture-shaped; without the
      // texture they'd be solid cards, so they're left out.
      m = new THREE.MeshBasicMaterial({ visible: false })
    } else if (z.kind === 2) {
      m = new THREE.MeshLambertMaterial({
        color: 0x3b82c4, transparent: true, opacity: 0.45, depthWrite: false, side: THREE.DoubleSide,
      })
    } else {
      const height = prefs.style === 'height'
      m = new THREE.MeshLambertMaterial({
        vertexColors: height,
        // Walls a shade darker than floors, so they read apart.
        color: height ? (faded ? 0x9aa0a8 : 0xffffff) : faded ? 0x6b7280 : 0xb3ac9c,
        flatShading: true,
        side,
      })
    }
    m.userData = { opacity: m.opacity, blends: m.transparent }
    L.materials.push(m)
    return m
  }
  // Floors share one array and walls and ceilings another, across the zone
  // and its objects; each layer only draws the groups it has.
  const n = L.data.materials.length
  const solid = Array.from({ length: n }, (_, i) => make(i, false))
  const faded = Array.from({ length: n }, (_, i) => make(i, true))
  L.faded = faded
  for (const layers of [L.own, L.objects]) {
    layers.floor.material = solid
    layers.wall.material = faded
    layers.ceiling.material = faded
    // Faded walls draw after solid floors.
    layers.wall.renderOrder = 1
    layers.ceiling.renderOrder = 1
  }
}

// adjustLook applies wall opacity and what's shown to the materials and
// meshes already built. Opacity is a uniform; only crossing between solid
// and see-through changes a material's shader, and three.js caches those.
function adjustLook(L: ZoneLayers, prefs: Map3DPrefs): void {
  const op = prefs.wallOpacity
  for (const m of L.faded) {
    const { opacity, blends } = m.userData as { opacity: number; blends: boolean }
    m.opacity = opacity * op
    const transparent = blends || op < 1
    if (m.transparent !== transparent) {
      m.transparent = transparent
      m.depthWrite = !transparent
      m.needsUpdate = true
    }
  }
  L.own.ceiling.visible = !prefs.hideCeilings
  L.objects.floor.visible = prefs.showObjects
  L.objects.wall.visible = prefs.showObjects
  L.objects.ceiling.visible = prefs.showObjects && !prefs.hideCeilings
}

function resetView(s: Scene3D): void {
  if (!s.home) return
  s.controls.target.copy(s.home.target)
  s.camera.position.copy(s.home.position)
  s.controls.update()
}

function markerCentroid(markers: RaiderMarker[]): THREE.Vector3 | null {
  if (markers.length === 0) return null
  const c = new THREE.Vector3()
  for (const r of markers) c.add(new THREE.Vector3(r.x, -r.y, r.z))
  return c.divideScalar(markers.length)
}

// Marker and pin geometry and materials are shared per size/colour and
// reused across frames, so playback at 20 fps doesn't churn GPU buffers.
const sphereGeo = new Map<number, THREE.BufferGeometry>()
const diamondGeo = new Map<number, THREE.BufferGeometry>()
const overMat = new Map<string, THREE.MeshBasicMaterial>()

function cached<K>(m: Map<K, THREE.BufferGeometry>, k: K, make: () => THREE.BufferGeometry): THREE.BufferGeometry {
  let g = m.get(k)
  if (!g) {
    g = make()
    m.set(k, g)
  }
  return g
}

function matFor(color: string): THREE.MeshBasicMaterial {
  let m = overMat.get(color)
  if (!m) {
    // Drawn over the walls, the same way the 2D map draws markers over
    // geometry: a raider behind a pillar should still be visible.
    m = new THREE.MeshBasicMaterial({ color, depthTest: false, transparent: true })
    overMat.set(color, m)
  }
  return m
}

function updateMarkers(s: Scene3D, markers: RaiderMarker[]): void {
  const g = s.markers
  while (g.children.length > markers.length) g.remove(g.children[g.children.length - 1])
  const size = s.markerSize
  markers.forEach((r, i) => {
    let m = g.children[i] as THREE.Mesh | undefined
    if (!m) {
      m = new THREE.Mesh(undefined, matFor(r.color))
      m.renderOrder = 10
      g.add(m)
    }
    const sz = r.focus ? size * 1.8 : size
    m.geometry = cached(sphereGeo, sz, () => new THREE.SphereGeometry(sz, 12, 8))
    m.material = matFor(r.dead ? '#6b7280' : r.color)
    // Raised a little: positions are at the feet.
    m.position.set(r.x, -r.y, r.z + size)
  })
}

function updatePins(s: Scene3D, pins: Pin3D[]): void {
  const g = s.pins
  while (g.children.length > pins.length) g.remove(g.children[g.children.length - 1])
  const size = s.markerSize * 1.3
  const geo = cached(diamondGeo, size, () => new THREE.OctahedronGeometry(size))
  pins.forEach((p, i) => {
    let m = g.children[i] as THREE.Mesh | undefined
    if (!m) {
      m = new THREE.Mesh(geo, matFor(p.color))
      m.renderOrder = 8
      g.add(m)
    }
    m.geometry = geo
    m.material = matFor(p.color)
    m.position.set(p.x, -p.y, p.z + size * 1.5)
  })
}

function updateTargetLine(s: Scene3D, t: ReplayTarget | null): void {
  const line = s.targetLine
  line.visible = t != null
  if (!t) return
  // Raised like the markers: positions are at the feet.
  const up = s.markerSize
  line.geometry.setFromPoints([
    new THREE.Vector3(t.from.x, -t.from.y, t.from.z + up),
    new THREE.Vector3(t.to.x, -t.to.y, t.to.z + up),
  ])
  line.computeLineDistances()
  const mat = line.material as THREE.LineDashedMaterial
  mat.dashSize = up * 1.5
  mat.gapSize = up
}

// drawLabels writes names over the WebGL canvas for markers and pins near
// the camera (and the focused marker at any distance).
function drawLabels(
  s: Scene3D,
  canvas: HTMLCanvasElement | null,
  w: number,
  h: number,
  markers: RaiderMarker[],
  pins: Pin3D[],
): void {
  if (!canvas) return
  const dpr = window.devicePixelRatio || 1
  if (canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(h * dpr)) {
    canvas.width = Math.round(w * dpr)
    canvas.height = Math.round(h * dpr)
  }
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, w, h)
  if (!s.home) return
  const size = s.markerSize
  const cam = s.camera.position
  const v = new THREE.Vector3()
  // Boxes of labels already drawn: one that would overlap is skipped, so a
  // packed raid shows a readable few rather than a smear.
  const taken: [number, number, number, number][] = []
  const put = (x: number, y: number, z: number, lift: number, text: string, color: string, bold: boolean): void => {
    v.set(x, -y, z + lift).project(s.camera)
    if (v.z > 1 || v.x < -1.1 || v.x > 1.1 || v.y < -1.1 || v.y > 1.1) return
    const px = (v.x + 1) * 0.5 * w
    const py = (1 - v.y) * 0.5 * h - 8
    ctx.font = bold ? '600 11px system-ui, sans-serif' : '10px system-ui, sans-serif'
    const half = ctx.measureText(text).width / 2 + 2
    const box: [number, number, number, number] = [px - half, py - 11, px + half, py + 3]
    if (taken.some((b) => box[0] < b[2] && box[2] > b[0] && box[1] < b[3] && box[3] > b[1])) return
    taken.push(box)
    ctx.textAlign = 'center'
    ctx.lineWidth = 3
    ctx.strokeStyle = 'rgba(0,0,0,0.75)'
    ctx.strokeText(text, px, py)
    ctx.fillStyle = color
    ctx.fillText(text, px, py)
  }
  // Pixels per world unit at distance 1, for the on-screen size of a marker.
  const pxPerUnit = h / 2 / Math.tan(THREE.MathUtils.degToRad(s.camera.fov / 2))
  const near = (x: number, y: number, z: number, across: number, minPx: number): boolean =>
    (across * pxPerUnit) / Math.max(1, cam.distanceTo(v.set(x, -y, z))) >= minPx
  // Focused marker first, so it always gets its label; then the others,
  // then pins.
  const ordered = [...markers].sort((a, b) => Number(!!b.focus) - Number(!!a.focus))
  for (const r of ordered) {
    if (r.focus || near(r.x, r.y, r.z, size * 2, LABEL_PX)) {
      put(r.x, r.y, r.z, size * 2.5, r.label, r.dead ? 'rgba(255,255,255,0.55)' : 'rgba(255,255,255,0.9)', !!r.focus)
    }
  }
  for (const p of pins) {
    if (p.label && near(p.x, p.y, p.z, size * 2.6, PIN_LABEL_PX)) put(p.x, p.y, p.z, size * 3, p.label, p.color, false)
  }
}
