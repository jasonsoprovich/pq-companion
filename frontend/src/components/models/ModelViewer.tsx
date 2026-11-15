import React, { useEffect, useRef, useState } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import { buildCharacter, buildItem, onCharacterTexture } from '../../lib/characterModels'
import type { CharacterModelRequest } from '../../lib/characterModels'
import type { CharacterModelRef, WornModel } from '../../services/api'
import type { ModelDress } from '../../lib/characterDress'

// ModelViewer shows one character model on its own, to turn around and zoom
// into, optionally dressed (the dressing room), or one item's model. three.js
// is large, so pages should load this lazily. It draws only when something
// changes (a drag, a texture arriving), so an untouched viewer costs no GPU
// time, and gives its WebGL context back as soon as it's closed.

export type { ModelDress } from '../../lib/characterDress'

/** DressInfo reports how a dressing went, for the page to explain. */
export interface DressInfo {
  ref: CharacterModelRef
  /** Slots with no texture for this model, left as they were. */
  missing: number[]
  /** Slots shown with a stand-in texture. */
  fallback: number[]
  /** Held items with no model in the client. */
  missingItems: ('primary' | 'secondary')[]
  /** What a Luclin model wears as models (hair, robe, plate). */
  wear: WornModel[]
}

type Status = 'loading' | 'ready' | 'none' | 'error' | 'webgl'

interface Stage {
  renderer: THREE.WebGLRenderer
  scene: THREE.Scene
  camera: THREE.PerspectiveCamera
  controls: OrbitControls
  render: () => void
  // The model and what it holds; replaced on every change.
  group: THREE.Group
  // What's shown, disposed when replaced.
  shown: { dispose: () => void } | null
  // The model framed last, so a change of outfit keeps the user's view.
  framed: string
}

export default function ModelViewer({ model, item, dress, onDressed, className }: {
  model?: CharacterModelRequest
  /** An item model to show instead (idfile, "IT10"). */
  item?: string
  dress?: ModelDress
  onDressed?: (info: DressInfo) => void
  className?: string
}): React.ReactElement {
  const host = useRef<HTMLDivElement>(null)
  const stage = useRef<Stage | null>(null)
  const [status, setStatus] = useState<Status>('loading')
  const [label, setLabel] = useState('')
  const { race = 0, gender = 0, zone, luclin } = model ?? {}
  const dressKey = dress ? JSON.stringify(dress) : ''
  const onDressedRef = useRef(onDressed)
  onDressedRef.current = onDressed

  // The renderer, camera and controls live as long as the viewer.
  useEffect(() => {
    const el = host.current
    if (!el) return
    let renderer: THREE.WebGLRenderer
    try {
      renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true })
    } catch {
      setStatus('webgl')
      return
    }
    // Past 2× the extra pixels cost more than they show.
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
    el.appendChild(renderer.domElement)

    const scene = new THREE.Scene()
    scene.add(new THREE.HemisphereLight(0xdfe6f0, 0x2a2622, 1.6))
    const key = new THREE.DirectionalLight(0xffffff, 1.4)
    key.position.set(-6, 10, 12)
    scene.add(key)
    const group = new THREE.Group()
    scene.add(group)

    const camera = new THREE.PerspectiveCamera(30, 1, 0.05, 500)
    camera.up.set(0, 0, 1)
    const controls = new OrbitControls(camera, renderer.domElement)
    controls.enableDamping = true
    controls.enablePan = false

    let frame = 0
    const render = (): void => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        controls.update()
        renderer.render(scene, camera)
      })
    }
    controls.addEventListener('change', render)
    const offTexture = onCharacterTexture(render)

    const resize = (): void => {
      const w = el.clientWidth || 1
      const h = el.clientHeight || 1
      renderer.setSize(w, h)
      camera.aspect = w / h
      camera.updateProjectionMatrix()
      render()
    }
    const ro = new ResizeObserver(resize)
    ro.observe(el)
    resize()
    stage.current = { renderer, scene, camera, controls, render, group, shown: null, framed: '' }

    return () => {
      cancelAnimationFrame(frame)
      offTexture()
      ro.disconnect()
      controls.dispose()
      stage.current?.shown?.dispose()
      stage.current = null
      renderer.dispose()
      // Browsers allow only a handful of WebGL contexts; don't wait for
      // garbage collection to give this one back.
      renderer.forceContextLoss()
      renderer.domElement.remove()
    }
  }, [])

  // An item on its own, framed whole whichever way it lies.
  useEffect(() => {
    const s = stage.current
    if (!s || !item) return
    let live = true
    setStatus('loading')
    ;(async () => {
      const it = await buildItem(item)
      if (!live) {
        it?.dispose()
        return
      }
      s.group.clear()
      s.shown?.dispose()
      s.shown = it
      if (!it) {
        setStatus('none')
        return
      }
      const box = new THREE.Box3().setFromObject(it.object)
      const centre = box.getCenter(new THREE.Vector3())
      it.object.position.sub(centre)
      const r = Math.max(box.getSize(new THREE.Vector3()).length() / 2, 0.1)
      s.group.add(it.object)
      s.controls.target.set(0, 0, 0)
      s.camera.position.set(r * 1.4, r * 3.6, r * 1.2)
      s.camera.near = r / 100
      s.camera.far = r * 100
      s.camera.updateProjectionMatrix()
      s.controls.minDistance = r * 0.8
      s.controls.maxDistance = r * 10
      s.controls.update()
      s.framed = ''
      setLabel(`${it.asset.ref.code} · ${it.asset.ref.archive}`)
      setStatus('ready')
      s.render()
    })().catch(() => live && setStatus('error'))
    return () => {
      live = false
    }
  }, [item])

  // The model, dressed.
  useEffect(() => {
    const s = stage.current
    if (!s || item || !model) return
    let live = true
    setStatus((st) => (st === 'ready' ? st : 'loading'))
    ;(async () => {
      const parsed: ModelDress | null = dressKey ? JSON.parse(dressKey) : null
      const c = await buildCharacter({ race, gender, zone, luclin }, parsed)
      if (!live) {
        c?.dispose()
        return
      }
      s.group.clear()
      s.shown?.dispose()
      s.shown = c
      if (!c) {
        setStatus('none')
        return
      }
      s.group.add(c.object)
      const refKey = `${c.base.ref.archive}|${c.base.ref.code}`
      if (s.framed !== refKey) {
        // Models face world +y (raw +x after the map-space swap and the y
        // flip), so the camera starts in front, a little to the side, back
        // far enough to fit the model's whole box in view: its height, or
        // its width (with a little of its length, seen from the side) on a
        // narrow viewer, plus half its length for the parts reaching
        // toward the camera (a hunched werewolf, a wolf's snout).
        const size = new THREE.Box3().setFromObject(c.object).getSize(new THREE.Vector3())
        const fit = Math.max(size.z, (size.x + size.y * 0.25) / s.camera.aspect)
        const half = THREE.MathUtils.degToRad(s.camera.fov) / 2
        const d = 1.3 * (fit / 2 / Math.tan(half) + size.y / 2)
        s.camera.near = Math.max(0.05, d / 1000)
        s.camera.far = d * 20
        s.camera.updateProjectionMatrix()
        s.controls.target.set(0, 0, size.z * 0.5)
        const dir = new THREE.Vector3(0.2, 1, 0.1).normalize()
        s.camera.position.copy(s.controls.target).addScaledVector(dir, d)
        s.controls.minDistance = d * 0.2
        s.controls.maxDistance = d * 3
        s.controls.update()
        s.framed = refKey
      }
      setLabel(`${c.base.ref.code} · ${c.base.ref.archive}`)
      setStatus('ready')
      s.render()
      onDressedRef.current?.({
        ref: c.base.ref, missing: c.missing, fallback: c.fallback, missingItems: c.missingItems,
        wear: c.wear,
      })
    })().catch(() => live && setStatus('error'))
    return () => {
      live = false
    }
    // model is read for whether it's set; its fields are the deps.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [race, gender, zone, luclin, dressKey, item, !model])

  const message: Partial<Record<Status, string>> = {
    loading: 'Loading model…',
    none: 'Your EverQuest client has no model for this.',
    error: 'The model couldn’t be loaded.',
    webgl: '3D needs WebGL, which isn’t available here.',
  }
  return (
    <div className={`relative ${className ?? ''}`}>
      <div ref={host} className="absolute inset-0" />
      {status !== 'ready' && (
        <div className="absolute inset-0 flex items-center justify-center text-xs" style={{ color: 'var(--color-muted)' }}>
          {message[status]}
        </div>
      )}
      {status === 'ready' && label && (
        <div className="pointer-events-none absolute bottom-1 right-2 text-[10px]" style={{ color: 'var(--color-muted)' }}>
          {label}
        </div>
      )}
    </div>
  )
}
