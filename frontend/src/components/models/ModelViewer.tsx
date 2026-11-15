import React, { useEffect, useRef, useState } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import {
  createCharacterMesh,
  loadCharacterModel,
  onCharacterTexture,
} from '../../lib/characterModels'
import type { CharacterModelRequest } from '../../lib/characterModels'

// ModelViewer shows one character model on its own, to turn around and zoom
// into: the base for the dressing room. three.js is large, so pages should
// load this lazily.

type Status = 'loading' | 'ready' | 'none' | 'error' | 'webgl'

export default function ModelViewer({ model, className }: {
  model: CharacterModelRequest
  className?: string
}): React.ReactElement {
  const host = useRef<HTMLDivElement>(null)
  const [status, setStatus] = useState<Status>('loading')
  const [label, setLabel] = useState('')
  const { race, gender, zone, luclin } = model

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

    let live = true
    setStatus('loading')
    loadCharacterModel({ race, gender, zone, luclin })
      .then((asset) => {
        if (!live) return
        if (!asset) {
          setStatus('none')
          return
        }
        const mesh = createCharacterMesh(asset)
        scene.add(mesh)
        // Models face world +y (raw +x after the map-space swap and the y
        // flip), so the camera starts in front, a little to the side.
        const h = asset.height
        controls.target.set(0, 0, h * 0.5)
        camera.position.set(h * 0.55, h * 2.6, h * 0.75)
        controls.minDistance = h * 0.6
        controls.maxDistance = h * 8
        controls.update()
        setLabel(`${asset.ref.code} · ${asset.ref.archive}`)
        setStatus('ready')
        render()
      })
      .catch(() => live && setStatus('error'))

    return () => {
      live = false
      cancelAnimationFrame(frame)
      offTexture()
      ro.disconnect()
      controls.dispose()
      renderer.dispose()
      renderer.domElement.remove()
    }
  }, [race, gender, zone, luclin])

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
