import React, { useEffect, useRef, useState } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import { getZoneMesh3D } from '../../services/api'
import type { RaiderMarker, ReplayTarget } from './ZoneMap'

// ZoneMap3D draws a zone in 3D from the user's own installed client files
// (<eq_path>/<zone>.s3d, read by the backend): solid geometry shaded by
// height, untextured, with raid replay markers on top. Loaded lazily, so
// three.js only downloads when someone opens the 3D view.
//
// World axes: map space x, -y, z, with z up. Map space y grows downward on
// the 2D map, so negating it keeps north at the top of the default view and
// left/right the same as the 2D map.

export interface ZoneMap3DProps {
  zone: string
  raiders: RaiderMarker[]
  // target draws a line from the observer to their target.
  target?: ReplayTarget | null
  // onUnavailable fires when 3D can't be shown (no WebGL, no s3d), so the
  // caller can fall back to 2D.
  onUnavailable?: (reason: string) => void
}

const LOW = new THREE.Color('#1e3a5f')
const HIGH = new THREE.Color('#e0a84c')

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
  for (let i = 0; i < pos.length; i += 3) {
    c.lerpColors(LOW, HIGH, (pos[i + 2] - lo) / span)
    out[i] = c.r
    out[i + 1] = c.g
    out[i + 2] = c.b
  }
  return out
}

interface Scene3D {
  renderer: THREE.WebGLRenderer
  scene: THREE.Scene
  camera: THREE.PerspectiveCamera
  controls: OrbitControls
  markers: THREE.Group
  targetLine: THREE.Line
  markerSize: number
  render: () => void
  dispose: () => void
}

export default function ZoneMap3D({ zone, raiders, target, onUnavailable }: ZoneMap3DProps): React.ReactElement {
  const host = useRef<HTMLDivElement>(null)
  const scene = useRef<Scene3D | null>(null)
  const raidersRef = useRef(raiders)
  raidersRef.current = raiders
  const [status, setStatus] = useState<'loading' | 'ready' | string>('loading')
  const lastFocus = useRef<THREE.Vector3 | null>(null)

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
    const markers = new THREE.Group()
    markers.renderOrder = 10
    sc.add(markers)
    const targetLine = new THREE.Line(
      new THREE.BufferGeometry(),
      new THREE.LineDashedMaterial({ color: 0xf87171, dashSize: 6, gapSize: 4, depthTest: false, transparent: true }),
    )
    targetLine.renderOrder = 9
    targetLine.visible = false
    sc.add(targetLine)

    let frame = 0
    const render = (): void => {
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
      })
    }
    controls.addEventListener('change', render)
    const ro = new ResizeObserver(render)
    ro.observe(el)

    const s: Scene3D = {
      renderer, scene: sc, camera, controls, markers, targetLine, markerSize: 4, render,
      dispose: () => {
        cancelAnimationFrame(frame)
        ro.disconnect()
        controls.dispose()
        // Marker geometry and materials are shared module-wide; only the
        // zone's own mesh is freed.
        sc.remove(markers)
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
        // World y is map y negated (see the header comment).
        const pos = new Float32Array(mesh.positions)
        for (let i = 1; i < pos.length; i += 3) pos[i] = -pos[i]
        const geo = new THREE.BufferGeometry()
        geo.setAttribute('position', new THREE.BufferAttribute(pos, 3))
        geo.setAttribute('color', new THREE.BufferAttribute(heightColors(pos), 3))
        geo.setIndex(new THREE.BufferAttribute(mesh.indices, 1))
        geo.computeVertexNormals()
        geo.computeBoundingBox()
        const mat = new THREE.MeshStandardMaterial({
          vertexColors: true, flatShading: true, roughness: 1, metalness: 0, side: THREE.DoubleSide,
        })
        sc.add(new THREE.Mesh(geo, mat))

        const box = geo.boundingBox!
        const center = box.getCenter(new THREE.Vector3())
        const extent = box.getSize(new THREE.Vector3())
        const radius = Math.max(extent.x, extent.y, 1)
        s.markerSize = Math.max(3, radius / 250)
        // Start over the raid if there is one, else over the zone, looking
        // north and down at about 50 degrees.
        const target = raidCentroid(raidersRef.current) ?? center
        const dist = raidersRef.current.length ? radius * 0.35 : radius * 0.9
        controls.target.copy(target)
        camera.position.set(target.x, target.y - dist * 0.65, target.z + dist * 0.75)
        camera.near = Math.max(1, radius / 5000)
        camera.far = radius * 10
        camera.updateProjectionMatrix()
        controls.update()
        setStatus('ready')
        updateMarkers(s, raidersRef.current)
        render()
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

  // Markers follow the playhead; the camera follows a focused raider.
  useEffect(() => {
    const s = scene.current
    if (!s || status !== 'ready') return
    updateMarkers(s, raiders)
    updateTargetLine(s, target ?? null)
    const f = raiders.find((r) => r.focus)
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
  }, [raiders, target, status])

  return (
    <div className="relative h-full w-full overflow-hidden rounded" style={{ backgroundColor: '#0b0d12' }}>
      <div ref={host} className="absolute inset-0" />
      {status !== 'ready' && (
        <div className="absolute inset-0 flex items-center justify-center p-6 text-center text-sm" style={{ color: 'var(--color-muted)' }}>
          {status === 'loading' ? 'Loading 3D zone…' : status}
        </div>
      )}
      {status === 'ready' && (
        <div className="pointer-events-none absolute bottom-2 left-2 text-[10px]" style={{ color: 'rgba(255,255,255,0.45)' }}>
          Drag to orbit · right-drag to pan · scroll to zoom
        </div>
      )}
    </div>
  )
}

function raidCentroid(raiders: RaiderMarker[]): THREE.Vector3 | null {
  if (raiders.length === 0) return null
  const c = new THREE.Vector3()
  for (const r of raiders) c.add(new THREE.Vector3(r.x, -r.y, r.z))
  return c.divideScalar(raiders.length)
}

// Marker geometry and materials are shared per size/colour and reused
// across frames, so playback at 20 fps doesn't churn GPU buffers.
const markerGeo = new Map<number, THREE.BufferGeometry>()
const markerMat = new Map<string, THREE.MeshBasicMaterial>()

function geoFor(size: number): THREE.BufferGeometry {
  let g = markerGeo.get(size)
  if (!g) {
    g = new THREE.SphereGeometry(size, 12, 8)
    markerGeo.set(size, g)
  }
  return g
}

function matFor(color: string): THREE.MeshBasicMaterial {
  let m = markerMat.get(color)
  if (!m) {
    // Drawn over the walls, the same way the 2D map draws raiders over
    // geometry: a raider behind a pillar should still be visible.
    m = new THREE.MeshBasicMaterial({ color, depthTest: false, transparent: true })
    markerMat.set(color, m)
  }
  return m
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

function updateMarkers(s: Scene3D, raiders: RaiderMarker[]): void {
  const g = s.markers
  while (g.children.length > raiders.length) g.remove(g.children[g.children.length - 1])
  const size = s.markerSize
  raiders.forEach((r, i) => {
    let m = g.children[i] as THREE.Mesh | undefined
    if (!m) {
      m = new THREE.Mesh(geoFor(size), matFor(r.color))
      m.renderOrder = 10
      g.add(m)
    }
    m.geometry = geoFor(r.focus ? size * 1.8 : size)
    m.material = matFor(r.dead ? '#6b7280' : r.color)
    // Raised a little: positions are at the feet.
    m.position.set(r.x, -r.y, r.z + size)
  })
}
