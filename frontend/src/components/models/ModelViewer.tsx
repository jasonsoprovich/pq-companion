import React, { useEffect, useRef, useState } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import { buildCharacter, onCharacterTexture } from '../../lib/characterModels'
import type { DressedCharacter, CharacterModelRequest } from '../../lib/characterModels'
import type { CharacterModelRef, WornModel } from '../../services/api'
import type { ModelDress } from '../../lib/characterDress'

// ModelViewer shows one character model on its own, to turn around and zoom
// into, optionally dressed (the dressing room). three.js is large, so pages
// should load this lazily.

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
  // The character shown, disposed when replaced.
  shown: DressedCharacter | null
  // The model framed last, so a change of outfit keeps the user's view.
  framed: string
}

export default function ModelViewer({ model, dress, onDressed, className }: {
  model: CharacterModelRequest
  dress?: ModelDress
  onDressed?: (info: DressInfo) => void
  className?: string
}): React.ReactElement {
  const host = useRef<HTMLDivElement>(null)
  const stage = useRef<Stage | null>(null)
  const [status, setStatus] = useState<Status>('loading')
  const [label, setLabel] = useState('')
  const { race, gender, zone, luclin } = model
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
    renderer.setPixelRatio(window.devicePixelRatio)
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
      renderer.domElement.remove()
    }
  }, [])

  // The model, dressed.
  useEffect(() => {
    const s = stage.current
    if (!s) return
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
        // flip), so the camera starts in front, a little to the side.
        const h = c.asset.height
        s.controls.target.set(0, 0, h * 0.5)
        s.camera.position.set(h * 0.55, h * 2.6, h * 0.75)
        s.controls.minDistance = h * 0.6
        s.controls.maxDistance = h * 8
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
  }, [race, gender, zone, luclin, dressKey])

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
