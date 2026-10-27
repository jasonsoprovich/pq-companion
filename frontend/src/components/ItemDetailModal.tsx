import React, { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { X } from 'lucide-react'
import type { Item } from '../types/item'
import ItemDetailView from './ItemDetailView'

interface ItemDetailModalProps {
  item: Item | null
  open: boolean
  onClose: () => void
}

export default function ItemDetailModal({ item, open, onClose }: ItemDetailModalProps): React.ReactElement | null {
  const navigate = useNavigate()

  useEffect(() => {
    if (!open) return
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open, onClose])

  if (!open || !item) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      onClick={onClose}
    >
      <div
        className="flex flex-col rounded-lg shadow-2xl w-full max-w-xl max-h-[80vh]"
        style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
        onClick={(e) => e.stopPropagation()}
      >
        <ItemDetailView
          item={item}
          onNavigate={onClose}
          onTitleClick={() => {
            navigate(`/items?select=${item.id}`)
            onClose()
          }}
          headerRight={
            <button onClick={onClose} title="Close">
              <X size={16} style={{ color: 'var(--color-muted)' }} />
            </button>
          }
        />
      </div>
    </div>
  )
}
