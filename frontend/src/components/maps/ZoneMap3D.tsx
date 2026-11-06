import React, { useEffect, useRef, useState } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import { mergeGeometries } from 'three/addons/utils/BufferGeometryUtils.js'
import { RotateCcw, SlidersHorizontal, Tag } from 'lucide-react'
import { getZoneMesh3D, getZoneTextureUrl } from '../../services/api'
import type { ZoneMesh3D } from '../../services/api'
import { useMap3DPrefs } from '../../hooks/useMap3DPrefs'
import type { Map3DLabels, Map3DPrefs, Map3DStyle } from '../../hooks/useMap3DPrefs'
import { drawCompass, type AggroRing, type RaiderMarker, type ReplayTarget, type SpawnDot } from './ZoneMap'

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
// or a kill mark in a replay. In map space, like markers. picked is a
// search result or the clicked pin, labelled whatever Labels is set to.
export interface Pin3D {
  x: number
  y: number
  z: number
  color: string
  label: string
  picked?: boolean
  // focus flies the camera to this pin when it first appears: a search
  // result or a clicked pin.
  focus?: boolean
}

// Camera3D is how the camera moves: freely, looking straight down, or
// riding behind whoever it follows, facing their way.
export type Camera3D = 'free' | 'top' | 'chase'

export interface ZoneMap3DProps {
  zone: string
  // markers are raiders in a replay, or you and your group live. The camera
  // follows the one with focus set.
  markers: RaiderMarker[]
  // target draws a line from the observer to their target.
  target?: ReplayTarget | null
  pins?: Pin3D[]
  // rings are spawn points' aggro radii, drawn flat on the floor, and
  // spawnDots the spawn points themselves; both as the 2D map has them.
  rings?: AggroRing[]
  spawnDots?: SpawnDot[]
  // For the map overlay: chromeless drops the buttons and hints, labels
  // overrides the shared Labels setting, and transparent lets the window
  // behind show through.
  chromeless?: boolean
  labels?: Map3DLabels
  transparent?: boolean
  // camera sets the camera mode from outside (the overlay); otherwise the
  // view's own buttons do. fitSeq frames every marker each time it changes.
  camera?: Camera3D
  fitSeq?: number
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

// Markers, pins and spawn dots stay the same size on screen at any zoom,
// like the 2D map's, so a packed raid reads as arrows rather than blobs.
// Sizes are in CSS pixels.
const PIN_PX = 5
const DOT_PX = 9
const DOT_UNKNOWN_PX = 6
const HOVER_PX = 10
const HIGHLIGHT = '#ff4fd8'

// Scrolling in never gets closer to the orbit point than the zone's size
// over CLOSEST: past that the point is pushed ahead, so zooming keeps
// moving instead of crawling to a stop. FLY_SPEED is keyboard movement in
// zone sizes per second.
const CLOSEST = 25
const FLY_SPEED = 1 / 8

const LABEL_NAMES: Record<Map3DLabels, string> = { off: 'Off', group: 'Group', all: 'All' }
const LABEL_NEXT: Record<Map3DLabels, Map3DLabels> = { off: 'group', group: 'all', all: 'off' }

// Hover is the marker, pin or spawn dot under the mouse, named over it.
interface Hover {
  x: number
  y: number
  z: number
  text: string
  color: string
}

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
  ringFill: THREE.Mesh
  ringLine: THREE.LineSegments
  dotsKnown: THREE.Points
  dotsOther: THREE.Points
  targetLine: THREE.Line
  layers: ZoneLayers | null
  // markerSize is a nominal marker size in world units, for when names
  // show and the target line's dashes. radius is the zone's size.
  markerSize: number
  radius: number
  hover: Hover | null
  // home is the starting camera, for Reset view.
  home: { position: THREE.Vector3; target: THREE.Vector3 } | null
  render: () => void
  dispose: () => void
}

export default function ZoneMap3D({
  zone, markers, target, pins, rings, spawnDots, chromeless, labels, transparent, camera, fitSeq,
  onUnavailable,
}: ZoneMap3DProps): React.ReactElement {
  const host = useRef<HTMLDivElement>(null)
  const labelCanvas = useRef<HTMLCanvasElement>(null)
  const scene = useRef<Scene3D | null>(null)
  const markersRef = useRef(markers)
  markersRef.current = markers
  const pinsRef = useRef(pins)
  pinsRef.current = pins
  const ringsRef = useRef(rings)
  ringsRef.current = rings
  const dotsRef = useRef(spawnDots)
  dotsRef.current = spawnDots
  const [status, setStatus] = useState<'loading' | 'ready' | string>('loading')
  const lastFocus = useRef<THREE.Vector3 | null>(null)
  const [prefs, setPrefs] = useMap3DPrefs()
  const prefsRef = useRef(prefs)
  prefsRef.current = prefs
  const labelMode = labels ?? prefs.labels
  const labelsRef = useRef(labelMode)
  labelsRef.current = labelMode
  const transparentRef = useRef(transparent)
  transparentRef.current = transparent
  const [showOptions, setShowOptions] = useState(false)
  const [camOwn, setCamOwn] = useState<Camera3D>('free')
  const cam = camera ?? camOwn
  const camRef = useRef(cam)
  camRef.current = cam
  // Moving the camera by hand (dragging, flying) leaves Chase, unless the
  // mode is set from outside.
  const leaveChase = useRef<() => void>(() => {})
  leaveChase.current = () => {
    if (camera === undefined && camRef.current === 'chase') setCamOwn('free')
  }

  // Build the scene once per zone.
  useEffect(() => {
    const el = host.current
    if (!el) return
    let cancelled = false
    let renderer: THREE.WebGLRenderer
    try {
      renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true })
    } catch {
      setStatus('3D needs WebGL, which isn’t available here.')
      onUnavailable?.('webgl')
      return
    }
    renderer.setPixelRatio(window.devicePixelRatio)
    renderer.setClearColor(0x0b0d12, transparentRef.current ? 0 : 1)
    el.appendChild(renderer.domElement)
    renderer.domElement.style.display = 'block'

    const sc = new THREE.Scene()
    const camera = new THREE.PerspectiveCamera(50, 1, 1, 100000)
    camera.up.set(0, 0, 1)
    const controls = new OrbitControls(camera, renderer.domElement)
    controls.screenSpacePanning = true
    // Zoom toward what's under the mouse, not the middle of the zone.
    controls.zoomToCursor = true
    controls.zoomSpeed = 1.5
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
    // Rings and spawn dots, like markers, are drawn over the walls, the
    // same way the 2D map draws them over geometry.
    const over = { transparent: true, depthTest: false, depthWrite: false, vertexColors: true }
    const ringFill = new THREE.Mesh(new THREE.BufferGeometry(), new THREE.MeshBasicMaterial({ ...over, side: THREE.DoubleSide }))
    ringFill.renderOrder = 5
    const ringLine = new THREE.LineSegments(new THREE.BufferGeometry(), new THREE.LineBasicMaterial(over))
    ringLine.renderOrder = 5
    const dot = (px: number): THREE.Points => {
      const p = new THREE.Points(
        new THREE.BufferGeometry(),
        new THREE.PointsMaterial({ ...over, size: px, sizeAttenuation: false, map: dotTexture(), alphaTest: 0.05 }),
      )
      p.renderOrder = 6
      return p
    }
    const dotsKnown = dot(DOT_PX)
    const dotsOther = dot(DOT_UNKNOWN_PX)
    sc.add(ringFill, ringLine, dotsOther, dotsKnown)
    const targetLine = new THREE.Line(
      new THREE.BufferGeometry(),
      new THREE.LineDashedMaterial({ color: 0xf87171, dashSize: 6, gapSize: 4, depthTest: false, transparent: true }),
    )
    targetLine.renderOrder = 9
    targetLine.visible = false
    sc.add(targetLine)

    let frame = 0
    const s: Scene3D = {
      renderer, scene: sc, camera, controls, markers: markerGroup, pins: pinGroup,
      ringFill, ringLine, dotsKnown, dotsOther, targetLine,
      layers: null, markerSize: 4, radius: 1000, hover: null, home: null,
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
          if (h > 0) scaleToScreen(s, h)
          renderer.render(sc, camera)
          drawLabels(s, labelCanvas.current, w, h, {
            markers: markersRef.current,
            pins: pinsRef.current ?? [],
            dots: dotsRef.current ?? [],
            labels: labelsRef.current,
          })
          drawNorth(s, labelCanvas.current, w, h)
        })
      },
      dispose: () => {
        cancelAnimationFrame(frame)
        cancelAnimationFrame(flying)
        ro.disconnect()
        controls.dispose()
        el.removeEventListener('keydown', onKeyDown)
        el.removeEventListener('keyup', onKeyUp)
        el.removeEventListener('blur', onBlur)
        el.removeEventListener('pointerdown', onPointerDown)
        renderer.domElement.removeEventListener('wheel', onWheel)
        renderer.domElement.removeEventListener('pointermove', onPointerMove)
        renderer.domElement.removeEventListener('pointerleave', onPointerLeave)
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

    // OrbitControls zooms by a fraction of the distance to its orbit point,
    // so near that point each scroll moves less and zoom seems to stop. On
    // a scroll in that gets too close, the point is pushed ahead along the
    // view. Runs after the controls' own wheel handler, registered first.
    const onWheel = (e: WheelEvent): void => {
      if (e.deltaY >= 0) return
      const closest = s.radius / CLOSEST
      const toTarget = controls.target.clone().sub(camera.position)
      if (toTarget.length() >= closest) return
      controls.target.copy(camera.position).addScaledVector(toTarget.normalize(), closest)
      controls.update()
    }
    renderer.domElement.addEventListener('wheel', onWheel, { passive: true })

    // WASD or the arrows fly the camera across the floor, Q and E down and
    // up, Shift faster. Keys only count while the map has focus, which a
    // click on it gives, so typing in the search box never moves it.
    el.tabIndex = 0
    el.style.outline = 'none'
    const keys = new Set<string>()
    let fast = false
    let flying = 0
    let lastStep = 0
    const fly = (now: number): void => {
      if (keys.size === 0) {
        flying = 0
        lastStep = 0
        return
      }
      const dt = lastStep ? Math.min(0.1, (now - lastStep) / 1000) : 1 / 60
      lastStep = now
      const on = (...codes: string[]): number => (codes.some((c) => keys.has(c)) ? 1 : 0)
      const ahead = on('KeyW', 'ArrowUp') - on('KeyS', 'ArrowDown')
      const side = on('KeyD', 'ArrowRight') - on('KeyA', 'ArrowLeft')
      const rise = on('KeyE') - on('KeyQ')
      // Forward is where the camera looks, flattened onto the floor; looking
      // straight down, it's the top of the screen.
      const fwd = camera.getWorldDirection(new THREE.Vector3())
      fwd.z = 0
      if (fwd.lengthSq() < 1e-6) fwd.set(0, 1, 0).applyQuaternion(camera.quaternion).setZ(0)
      fwd.normalize()
      const right = new THREE.Vector3(fwd.y, -fwd.x, 0)
      const move = fwd.multiplyScalar(ahead).addScaledVector(right, side)
      move.z = rise
      if (move.lengthSq() > 0) {
        move.normalize().multiplyScalar(s.radius * FLY_SPEED * (fast ? 3 : 1) * dt)
        camera.position.add(move)
        controls.target.add(move)
        controls.update()
      }
      flying = requestAnimationFrame(fly)
    }
    const FLY_KEYS = new Set([
      'KeyW', 'KeyA', 'KeyS', 'KeyD', 'KeyQ', 'KeyE', 'ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight',
    ])
    const onKeyDown = (e: KeyboardEvent): void => {
      fast = e.shiftKey
      if (!FLY_KEYS.has(e.code) || e.ctrlKey || e.metaKey || e.altKey) return
      // Arrows would otherwise scroll the page.
      e.preventDefault()
      leaveChase.current()
      keys.add(e.code)
      if (!flying) flying = requestAnimationFrame(fly)
    }
    const onKeyUp = (e: KeyboardEvent): void => {
      fast = e.shiftKey
      keys.delete(e.code)
    }
    const onBlur = (): void => keys.clear()
    const onPointerDown = (): void => el.focus({ preventScroll: true })
    el.addEventListener('keydown', onKeyDown)
    el.addEventListener('keyup', onKeyUp)
    el.addEventListener('blur', onBlur)
    el.addEventListener('pointerdown', onPointerDown)

    // Hovering names the marker, pin or spawn dot under the mouse.
    const onPointerMove = (e: PointerEvent): void => {
      if (e.buttons !== 0) leaveChase.current()
      if (e.buttons !== 0 || !s.home) return
      const r = el.getBoundingClientRect()
      const next = pick(s, r.width, r.height, e.clientX - r.left, e.clientY - r.top, {
        markers: markersRef.current,
        pins: pinsRef.current ?? [],
        dots: dotsRef.current ?? [],
      })
      if (next?.text !== s.hover?.text || next?.x !== s.hover?.x || next?.y !== s.hover?.y) {
        s.hover = next
        s.render()
      }
    }
    const onPointerLeave = (): void => {
      if (!s.hover) return
      s.hover = null
      s.render()
    }
    renderer.domElement.addEventListener('pointermove', onPointerMove)
    renderer.domElement.addEventListener('pointerleave', onPointerLeave)

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
        s.radius = radius
        controls.maxDistance = radius * 3
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
        updateRings(s, ringsRef.current ?? [])
        updateDots(s, dotsRef.current ?? [])
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
      if (camRef.current === 'chase') chaseBehind(s, f)
      s.controls.update()
    } else {
      lastFocus.current = null
    }
    s.render()
  }, [markers, target, status])

  // Top looks straight down from where the camera is; Chase swings it
  // behind whoever it follows on the next marker update, and now.
  useEffect(() => {
    const s = scene.current
    if (!s || status !== 'ready') return
    if (cam === 'top') lookDown(s)
    if (cam === 'chase') {
      const f = markersRef.current.find((r) => r.focus)
      if (f) chaseBehind(s, f)
    }
    s.controls.update()
    s.render()
  }, [cam, status])

  // Fit: frame every marker, keeping the direction the camera looks from.
  useEffect(() => {
    const s = scene.current
    if (!s || status !== 'ready' || fitSeq === undefined) return
    const ms = markersRef.current
    if (ms.length === 0) return
    const box = new THREE.Box3()
    for (const r of ms) box.expandByPoint(new THREE.Vector3(r.x, -r.y, r.z))
    const centre = box.getCenter(new THREE.Vector3())
    const size = box.getSize(new THREE.Vector3()).length()
    const fov = THREE.MathUtils.degToRad(s.camera.fov / 2)
    const dist = Math.max(s.radius / 40, (size / 2 / Math.tan(fov)) * 1.4)
    flyTo(s, centre, dist)
    lastFocus.current = null
  }, [fitSeq, status])

  // A new search result or clicked pin: fly over to it.
  const focusPin = pins?.find((p) => p.focus)
  const focusKey = focusPin ? `${focusPin.x},${focusPin.y},${focusPin.z},${focusPin.label}` : ''
  useEffect(() => {
    const s = scene.current
    if (!s || status !== 'ready' || !focusPin) return
    const dist = Math.min(s.camera.position.distanceTo(s.controls.target), s.radius / 6)
    flyTo(s, new THREE.Vector3(focusPin.x, -focusPin.y, focusPin.z), dist)
    leaveChase.current()
    // focusKey stands for the pin, whose object changes every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusKey, status])

  useEffect(() => {
    const s = scene.current
    if (!s || status !== 'ready') return
    updatePins(s, pins ?? [])
    s.render()
  }, [pins, status])

  useEffect(() => {
    const s = scene.current
    if (!s || status !== 'ready') return
    updateRings(s, rings ?? [])
    s.render()
  }, [rings, status])

  useEffect(() => {
    const s = scene.current
    if (!s || status !== 'ready') return
    updateDots(s, spawnDots ?? [])
    s.render()
  }, [spawnDots, status])

  // Names are drawn each frame from the current setting.
  useEffect(() => {
    scene.current?.render()
  }, [labelMode])

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
    <div
      className="relative h-full w-full overflow-hidden rounded"
      style={{ backgroundColor: transparent ? 'transparent' : '#0b0d12' }}
    >
      <div ref={host} className="absolute inset-0" />
      <canvas ref={labelCanvas} className="pointer-events-none absolute inset-0 h-full w-full" />
      {status !== 'ready' && (
        <div className="absolute inset-0 flex items-center justify-center p-6 text-center text-sm" style={{ color: 'var(--color-muted)' }}>
          {status === 'loading' ? 'Loading 3D zone…' : status}
        </div>
      )}
      {status === 'ready' && !chromeless && (
        <>
          <div className="absolute right-2 top-2 flex items-center gap-1">
            <button
              className="rounded border px-1.5 py-0.5 text-[10px]"
              style={{ backgroundColor: 'rgba(11,13,18,0.8)', borderColor: 'var(--color-border)', color: 'var(--color-muted-foreground)' }}
              title="Look straight down from here"
              onClick={() => {
                const s = scene.current
                if (!s) return
                setCamOwn('free')
                lookDown(s)
                s.controls.update()
                s.render()
              }}
            >
              Top
            </button>
            <button
              className="rounded border px-1.5 py-0.5 text-[10px]"
              style={{
                backgroundColor: cam === 'chase' ? 'var(--color-primary)' : 'rgba(11,13,18,0.8)',
                borderColor: cam === 'chase' ? 'var(--color-primary)' : 'var(--color-border)',
                color: cam === 'chase' ? 'var(--color-background)' : 'var(--color-muted-foreground)',
              }}
              title={
                'Ride behind you, facing the way you face (needs Follow me). ' +
                'Dragging or flying the camera turns it off.'
              }
              onClick={() => setCamOwn(cam === 'chase' ? 'free' : 'chase')}
            >
              Chase
            </button>
            <button
              className="flex items-center gap-1 rounded border px-1.5 py-0.5 text-[10px]"
              style={{ backgroundColor: 'rgba(11,13,18,0.8)', borderColor: 'var(--color-border)', color: 'var(--color-muted-foreground)' }}
              title={
                'Whose names show: Off (only who the camera follows), Group ' +
                '(you and your group) or All (everyone close enough to read). ' +
                'Hover anything to name it.'
              }
              onClick={() => setPrefs({ labels: LABEL_NEXT[prefs.labels] })}
            >
              <Tag size={10} /> Labels: {LABEL_NAMES[prefs.labels]}
            </button>
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
            Drag to orbit · right-drag to pan · scroll to zoom · click, then WASD/arrows to move, Q/E down/up, Shift faster
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
        <span>Objects &amp; doors</span>
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

// lookDown puts the camera straight above what it orbits, as far away as
// it is now. A hair off vertical, where the orbit controls are steady.
function lookDown(s: Scene3D): void {
  const d = s.camera.position.distanceTo(s.controls.target)
  s.camera.position.copy(s.controls.target).add(new THREE.Vector3(0, -d * 0.001, d))
}

// CHASE_PITCH is how far above the follow's back Chase sits.
const CHASE_PITCH = THREE.MathUtils.degToRad(35)

// chaseBehind orbits the camera to sit behind marker r, looking the way it
// faces, keeping the current distance. EQ's heading turns counter-clockwise
// from north (+y here), so facing is (-sin, cos).
function chaseBehind(s: Scene3D, r: RaiderMarker): void {
  const a = ((r.heading ?? 0) / 512) * Math.PI * 2
  const d = s.camera.position.distanceTo(s.controls.target)
  const back = new THREE.Vector3(Math.sin(a), -Math.cos(a), 0).multiplyScalar(d * Math.cos(CHASE_PITCH))
  s.controls.target.set(r.x, -r.y, r.z)
  s.camera.position.copy(s.controls.target).add(back).add(new THREE.Vector3(0, 0, d * Math.sin(CHASE_PITCH)))
}

// flyTo glides the camera over half a second to orbit at, dist away,
// looking from the direction it looks from now.
function flyTo(s: Scene3D, at: THREE.Vector3, dist: number): void {
  const fromTarget = s.controls.target.clone()
  const fromPos = s.camera.position.clone()
  const dir = fromPos.clone().sub(fromTarget).normalize()
  const toPos = at.clone().addScaledVector(dir, dist)
  const start = performance.now()
  const step = (now: number): void => {
    const k = Math.min(1, (now - start) / 500)
    const e = k * k * (3 - 2 * k)
    s.controls.target.lerpVectors(fromTarget, at, e)
    s.camera.position.lerpVectors(fromPos, toPos, e)
    s.controls.update()
    if (k < 1) requestAnimationFrame(step)
  }
  requestAnimationFrame(step)
}

// drawNorth draws the compass in the bottom-right corner, its needle on
// north (+y) as the camera sees it.
function drawNorth(s: Scene3D, canvas: HTMLCanvasElement | null, w: number, h: number): void {
  const ctx = canvas?.getContext('2d')
  if (!ctx || !s.home) return
  const t = s.controls.target
  const a = t.clone().project(s.camera)
  const b = t.clone().add(new THREE.Vector3(0, s.radius / 50, 0)).project(s.camera)
  const dx = b.x - a.x
  const dy = b.y - a.y
  // Looking straight along north there's no direction to show.
  if (Math.hypot(dx * w, dy * h) < 1e-3) return
  drawCompass(ctx, w - 24, h - 28, Math.atan2(dx * w, dy * h))
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

// Marker and pin geometry and materials are shared module-wide and reused
// across frames, so playback at 20 fps doesn't churn GPU buffers. Shapes are
// in screen pixels at scale 1; scaleToScreen sizes them each frame.
const overMat = new Map<string, THREE.MeshBasicMaterial>()

function matFor(color: string, opacity = 1): THREE.MeshBasicMaterial {
  const key = `${color}|${opacity}`
  let m = overMat.get(key)
  if (!m) {
    // Drawn over the walls, the same way the 2D map draws markers over
    // geometry: a raider behind a pillar should still be visible.
    m = new THREE.MeshBasicMaterial({ color, opacity, depthTest: false, transparent: true, side: THREE.DoubleSide })
    overMat.set(key, m)
  }
  return m
}

// The 2D map's raider arrow, pointing north, and a slightly larger copy
// behind it as its dark casing.
const CHEVRON: [number, number][] = [[0, 6], [4, -5], [0, -2.5], [-4, -5]]

function polygon(pts: [number, number][], grow: number): THREE.BufferGeometry {
  return new THREE.ShapeGeometry(new THREE.Shape(pts.map(([x, y]) => new THREE.Vector2(x * grow, y * grow))))
}

// cross is a dead raider's X, as the 2D map draws it; arm is its half
// thickness.
function cross(arm: number): THREE.BufferGeometry {
  const bar = (turn: number): THREE.BufferGeometry => new THREE.PlaneGeometry(11.5, arm * 2).rotateZ(turn)
  return mergeGeometries([bar(Math.PI / 4), bar(-Math.PI / 4)])
}

let shapes: Record<'chevron' | 'chevronCase' | 'cross' | 'crossCase' | 'ring' | 'ringCase' | 'pin', THREE.BufferGeometry> | null = null

function markerShapes(): NonNullable<typeof shapes> {
  if (!shapes) {
    shapes = {
      chevron: polygon(CHEVRON, 1),
      chevronCase: polygon(CHEVRON, 1.35),
      cross: cross(1),
      crossCase: cross(2),
      ring: new THREE.RingGeometry(9.25, 10.75, 32),
      ringCase: new THREE.RingGeometry(8.25, 11.75, 32),
      pin: new THREE.OctahedronGeometry(1),
    }
  }
  return shapes
}

// A marker is a group of four meshes: casing, shape, and the focus ring's
// casing and ring. userData holds where it stands and its size, for
// scaleToScreen.
function updateMarkers(s: Scene3D, markers: RaiderMarker[]): void {
  const g = s.markers
  const sh = markerShapes()
  while (g.children.length > markers.length) g.remove(g.children[g.children.length - 1])
  markers.forEach((r, i) => {
    let m = g.children[i] as THREE.Group | undefined
    if (!m) {
      m = new THREE.Group()
      for (let k = 0; k < 4; k++) {
        const mesh = new THREE.Mesh()
        // Casings under shapes, focus rings over everything.
        mesh.renderOrder = [10, 11, 12, 13][k]
        m.add(mesh)
      }
      m.userData = { base: new THREE.Vector3(), px: 1, lift: 0 }
      g.add(m)
    }
    const [casing, shape, ringCase, ring] = m.children as THREE.Mesh[]
    casing.geometry = r.dead ? sh.crossCase : sh.chevronCase
    casing.material = matFor('#000000', 0.7)
    shape.geometry = r.dead ? sh.cross : sh.chevron
    shape.material = matFor(r.color, r.dead ? 0.7 : 1)
    ringCase.geometry = sh.ringCase
    ringCase.material = matFor('#000000', 0.7)
    ring.geometry = sh.ring
    ring.material = matFor(HIGHLIGHT)
    ringCase.visible = !!r.focus
    ring.visible = !!r.focus
    // Lying flat at the feet, pointing along the heading: EQ's heading
    // turns counter-clockwise from north, as rotation about +z does here.
    // The ring is a child too, so it turns with it, which a circle hides.
    m.rotation.z = r.dead ? 0 : ((r.heading ?? 0) / 512) * Math.PI * 2
    ;(m.userData.base as THREE.Vector3).set(r.x, -r.y, r.z + 1)
    m.userData.px = r.focus ? 1.3 : 1
  })
}

function updatePins(s: Scene3D, pins: Pin3D[]): void {
  const g = s.pins
  const geo = markerShapes().pin
  while (g.children.length > pins.length) g.remove(g.children[g.children.length - 1])
  pins.forEach((p, i) => {
    let m = g.children[i] as THREE.Mesh | undefined
    if (!m) {
      m = new THREE.Mesh(geo)
      m.renderOrder = 8
      m.userData = { base: new THREE.Vector3(), px: PIN_PX, lift: PIN_PX * 1.4 }
      g.add(m)
    }
    m.material = matFor(p.color)
    ;(m.userData.base as THREE.Vector3).set(p.x, -p.y, p.z)
    m.userData.px = p.picked ? PIN_PX * 1.4 : PIN_PX
  })
}

// RING_SEGMENTS is how many sides each aggro ring has.
const RING_SEGMENTS = 40

// updateRings draws every aggro ring as one mesh of faint discs and one set
// of outlines, coloured per vertex with alpha, matching the 2D map's light
// fill and stronger edge. Rebuilt whenever the rings change.
function updateRings(s: Scene3D, rings: AggroRing[]): void {
  const n = rings.length
  const seg = RING_SEGMENTS
  const fillPos = new Float32Array(n * (seg + 1) * 3)
  const fillCol = new Float32Array(n * (seg + 1) * 4)
  const fillIdx = new Uint32Array(n * seg * 3)
  const linePos = new Float32Array(n * seg * 3)
  const lineCol = new Float32Array(n * seg * 4)
  const lineIdx = new Uint32Array(n * seg * 2)
  const c = new THREE.Color()
  rings.forEach((r, i) => {
    c.set(r.color)
    const a = (r.faint ? 0.45 : 1) * (r.alpha ?? 1)
    const x = r.x
    const y = -r.y
    const z = r.z + 1
    const fv = i * (seg + 1)
    fillPos.set([x, y, z], fv * 3)
    fillCol.set([c.r, c.g, c.b, a * 0.1], fv * 4)
    for (let k = 0; k < seg; k++) {
      const t = (k / seg) * Math.PI * 2
      const px = x + Math.cos(t) * r.radius
      const py = y + Math.sin(t) * r.radius
      fillPos.set([px, py, z], (fv + 1 + k) * 3)
      fillCol.set([c.r, c.g, c.b, a * 0.1], (fv + 1 + k) * 4)
      fillIdx.set([fv, fv + 1 + k, fv + 1 + ((k + 1) % seg)], (i * seg + k) * 3)
      const lv = i * seg + k
      linePos.set([px, py, z], lv * 3)
      lineCol.set([c.r, c.g, c.b, a * 0.75], lv * 4)
      lineIdx.set([lv, i * seg + ((k + 1) % seg)], lv * 2)
    }
  })
  const build = (pos: Float32Array, col: Float32Array, idx: Uint32Array): THREE.BufferGeometry => {
    const geo = new THREE.BufferGeometry()
    geo.setAttribute('position', new THREE.BufferAttribute(pos, 3))
    geo.setAttribute('color', new THREE.BufferAttribute(col, 4))
    geo.setIndex(new THREE.BufferAttribute(idx, 1))
    return geo
  }
  s.ringFill.geometry.dispose()
  s.ringFill.geometry = build(fillPos, fillCol, fillIdx)
  s.ringLine.geometry.dispose()
  s.ringLine.geometry = build(linePos, lineCol, lineIdx)
}

// updateDots draws spawn points as screen-sized dots, those with a respawn
// timer larger, as the 2D map does.
function updateDots(s: Scene3D, dots: SpawnDot[]): void {
  const c = new THREE.Color()
  for (const [pts, known] of [[s.dotsKnown, true], [s.dotsOther, false]] as const) {
    const sel = dots.filter((d) => d.known === known)
    const pos = new Float32Array(sel.length * 3)
    const col = new Float32Array(sel.length * 4)
    sel.forEach((d, i) => {
      c.set(d.color)
      pos.set([d.x, -d.y, d.z + 1], i * 3)
      col.set([c.r, c.g, c.b, known ? 1 : 0.7], i * 4)
    })
    const geo = new THREE.BufferGeometry()
    geo.setAttribute('position', new THREE.BufferAttribute(pos, 3))
    geo.setAttribute('color', new THREE.BufferAttribute(col, 4))
    pts.geometry.dispose()
    pts.geometry = geo
  }
}

// dotTexture is a white disc with a dark edge, tinted per dot.
let dotTex: THREE.Texture | null = null

function dotTexture(): THREE.Texture {
  if (dotTex) return dotTex
  const c = document.createElement('canvas')
  c.width = 32
  c.height = 32
  const ctx = c.getContext('2d')
  if (ctx) {
    ctx.beginPath()
    ctx.arc(16, 16, 15, 0, Math.PI * 2)
    ctx.fillStyle = 'rgba(0,0,0,0.8)'
    ctx.fill()
    ctx.beginPath()
    ctx.arc(16, 16, 11, 0, Math.PI * 2)
    ctx.fillStyle = '#ffffff'
    ctx.fill()
  }
  dotTex = new THREE.CanvasTexture(c)
  dotTex.colorSpace = THREE.SRGBColorSpace
  return dotTex
}

// worldPerPx is how many world units one screen pixel covers at distance 1.
function worldPerPx(s: Scene3D, h: number): number {
  return (2 * Math.tan(THREE.MathUtils.degToRad(s.camera.fov / 2))) / h
}

// scaleToScreen sizes each marker and pin for its distance from the camera,
// so they stay the same size on screen, and lifts pins by a pixel amount.
function scaleToScreen(s: Scene3D, h: number): void {
  const k = worldPerPx(s, h)
  const cam = s.camera.position
  for (const o of [...s.markers.children, ...s.pins.children]) {
    const { base, px, lift } = o.userData as { base: THREE.Vector3; px: number; lift: number }
    const per = cam.distanceTo(base) * k
    o.scale.setScalar(per * px)
    o.position.set(base.x, base.y, base.z + per * lift)
  }
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

// Overlays is what's drawn over the zone, for names and hover.
interface Overlays {
  markers: RaiderMarker[]
  pins: Pin3D[]
  dots: SpawnDot[]
}

function dotText(d: SpawnDot): string {
  const when = !d.known ? 'no kill seen' : d.label === 'up' ? 'up' : `respawn in ${d.label}`
  return `${d.name ?? 'Spawn point'} · ${when}`
}

// pick returns what's under the mouse at (mx, my): the nearest marker, pin
// or spawn dot within HOVER_PX, markers first when two are as close.
function pick(s: Scene3D, w: number, h: number, mx: number, my: number, o: Overlays): Hover | null {
  const v = new THREE.Vector3()
  let best: Hover | null = null
  let bestD = HOVER_PX * HOVER_PX
  const consider = (x: number, y: number, z: number, text: string, color: string): void => {
    v.set(x, -y, z).project(s.camera)
    if (v.z > 1) return
    const dx = (v.x + 1) * 0.5 * w - mx
    const dy = (1 - v.y) * 0.5 * h - my
    const d = dx * dx + dy * dy
    if (d < bestD) {
      bestD = d
      best = { x, y, z, text, color }
    }
  }
  for (const r of o.markers) consider(r.x, r.y, r.z + 1, r.dead ? `${r.label} (dead)` : r.label, 'rgba(255,255,255,0.95)')
  for (const p of o.pins) if (p.label) consider(p.x, p.y, p.z, p.label, p.color)
  for (const d of o.dots) consider(d.x, d.y, d.z + 1, dotText(d), 'rgba(255,255,255,0.95)')
  return best
}

// drawLabels writes names over the WebGL canvas: whatever is hovered, then
// markers by the Labels setting, then pins and respawn countdowns near the
// camera.
function drawLabels(
  s: Scene3D,
  canvas: HTMLCanvasElement | null,
  w: number,
  h: number,
  o: Overlays & { labels: Map3DLabels },
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
  // lift is how far above the point the text sits, in pixels.
  const put = (x: number, y: number, z: number, lift: number, text: string, color: string, bold: boolean): void => {
    v.set(x, -y, z).project(s.camera)
    if (v.z > 1 || v.x < -1.1 || v.x > 1.1 || v.y < -1.1 || v.y > 1.1) return
    const px = (v.x + 1) * 0.5 * w
    const py = (1 - v.y) * 0.5 * h - lift
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
  // Markers stay one size on screen, so "close enough to name" is judged
  // from a nominal world size instead.
  const pxPerUnit = 1 / worldPerPx(s, h)
  const near = (x: number, y: number, z: number, across: number, minPx: number): boolean =>
    (across * pxPerUnit) / Math.max(1, cam.distanceTo(v.set(x, -y, z))) >= minPx
  // The hovered thing first, so it always gets its label.
  if (s.hover) put(s.hover.x, s.hover.y, s.hover.z, 14, s.hover.text, s.hover.color, true)
  // Then the followed marker, then your group, then the rest.
  const rank = (r: RaiderMarker): number => (r.focus ? 0 : r.mine ? 1 : 2)
  const ordered = [...o.markers].sort((a, b) => rank(a) - rank(b))
  for (const r of ordered) {
    const show =
      r.focus ||
      (o.labels !== 'off' && r.mine) ||
      (o.labels === 'all' && near(r.x, r.y, r.z, size * 2, LABEL_PX))
    if (show) {
      put(r.x, r.y, r.z + 1, r.focus ? 17 : 11, r.label, r.dead ? 'rgba(255,255,255,0.55)' : 'rgba(255,255,255,0.9)', !!r.focus)
    }
  }
  for (const p of o.pins) {
    if (!p.label) continue
    if (p.picked || (o.labels !== 'off' && near(p.x, p.y, p.z, size * 2.6, PIN_LABEL_PX))) {
      put(p.x, p.y, p.z, PIN_PX * 3.4, p.label, p.color, !!p.picked)
    }
  }
  // Respawn countdowns once close; names wait for a hover.
  if (o.labels === 'off') return
  for (const d of o.dots) {
    if (d.known && d.label && near(d.x, d.y, d.z, size * 2, LABEL_PX)) {
      put(d.x, d.y, d.z + 1, 8, d.label, 'rgba(255,255,255,0.7)', false)
    }
  }
}
