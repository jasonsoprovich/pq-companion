import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ReactFlow,
  Background,
  Handle,
  Position,
  MarkerType,
  useNodesState,
  useEdgesState,
  type Node,
  type Edge,
  type NodeMouseHandler,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import dagre from 'dagre'
import type { PoPFlagStatus } from '../types/popflag'
import { stepKindMeta } from '../lib/popFlagKind'

// PoPFlagGraphPanel renders the flag dependency DAG: nodes are flags coloured
// by effective status, edges are prereqs (prereq → flag). Reuses the same
// @xyflow/react + dagre stack as SchemaGraphPanel. Structure is static (the
// dataset never changes shape), so layout runs once; only node colour updates
// as the character's state changes.

type Status = 'done' | 'available' | 'locked'

const STATUS_COLORS: Record<Status, { border: string; bg: string; label: string }> = {
  done: { border: '#34d399', bg: 'rgba(52,211,153,0.14)', label: 'Done' },
  available: { border: '#60a5fa', bg: 'rgba(96,165,250,0.12)', label: 'Available' },
  locked: { border: '#f87171', bg: 'rgba(248,113,113,0.08)', label: 'Locked' },
}

function statusOf(f: PoPFlagStatus): Status {
  if (f.done) return 'done'
  if (f.locked) return 'locked'
  return 'available'
}

const NODE_WIDTH = 184
const NODE_HEIGHT = 48

function layoutGraph(nodes: Node[], edges: Edge[]): Node[] {
  const g = new dagre.graphlib.Graph()
  g.setGraph({ rankdir: 'LR', nodesep: 24, ranksep: 80, marginx: 20, marginy: 20 })
  g.setDefaultEdgeLabel(() => ({}))
  for (const n of nodes) g.setNode(n.id, { width: NODE_WIDTH, height: NODE_HEIGHT })
  for (const e of edges) g.setEdge(e.source, e.target)
  dagre.layout(g)
  return nodes.map((n) => {
    const pos = g.node(n.id)
    return { ...n, position: { x: pos.x - NODE_WIDTH / 2, y: pos.y - NODE_HEIGHT / 2 } }
  })
}

interface FlagNodeData {
  label: string
  zoneShort: string
  status: Status
  title: string
  kind?: string // step_kind — drives the icon/accent badge (static per node)
  dimmed?: boolean
  highlighted?: boolean
  [key: string]: unknown
}

const hiddenHandleStyle: React.CSSProperties = {
  opacity: 0, width: 1, height: 1, minWidth: 0, minHeight: 0,
  border: 'none', background: 'transparent', pointerEvents: 'none',
}

function FlagNode({ data }: { data: FlagNodeData }): React.ReactElement {
  const palette = STATUS_COLORS[data.status]
  const km = stepKindMeta(data.kind)
  const KindIcon = km?.icon
  // Highlight ring wins; otherwise timed-hail nodes get a warm glow so the
  // easy-to-miss "act now after the kill" steps stand out in the graph.
  const boxShadow = data.highlighted
    ? `0 0 0 2px ${palette.border}`
    : km?.kind === 'timed_hail'
      ? `0 0 9px 1px ${km.color}66`
      : 'none'
  return (
    <div
      title={data.title}
      style={{
        width: NODE_WIDTH,
        minHeight: NODE_HEIGHT,
        backgroundColor: palette.bg,
        border: `1px solid ${data.highlighted ? '#fff' : palette.border}`,
        borderLeft: `3px solid ${km ? km.color : palette.border}`,
        borderRadius: 5,
        opacity: data.dimmed ? 0.18 : 1,
        transition: 'opacity 0.15s ease, border-color 0.15s ease',
        boxShadow,
        padding: '6px 9px',
        fontSize: 11,
        display: 'flex',
        flexDirection: 'column',
        gap: 2,
      }}
    >
      <Handle type="target" position={Position.Left} style={hiddenHandleStyle} isConnectable={false} />
      <Handle type="source" position={Position.Right} style={hiddenHandleStyle} isConnectable={false} />
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 5 }}>
        {KindIcon && (
          <KindIcon size={12} style={{ color: km!.color, flexShrink: 0, marginTop: 1 }} />
        )}
        <span
          style={{
            color: '#e5e7eb',
            fontWeight: 600,
            lineHeight: 1.2,
            flex: 1,
            textDecoration: data.status === 'done' ? 'line-through' : 'none',
          }}
        >
          {data.label}
        </span>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 4 }}>
        <span style={{ color: palette.border, fontSize: 9, textTransform: 'uppercase', letterSpacing: 0.5 }}>
          {data.zoneShort}
        </span>
        {km && (
          <span
            style={{
              color: km.color,
              fontSize: 8,
              textTransform: 'uppercase',
              letterSpacing: 0.5,
              padding: '1px 4px',
              borderRadius: 3,
              backgroundColor: km.bg,
              border: `1px solid ${km.border}`,
              whiteSpace: 'nowrap',
            }}
          >
            {km.label}
          </span>
        )}
      </div>
    </div>
  )
}

const nodeTypes = { flag: FlagNode }

interface PoPFlagGraphPanelProps {
  flags: PoPFlagStatus[]
}

export default function PoPFlagGraphPanel({ flags }: PoPFlagGraphPanelProps): React.ReactElement {
  const labelById = useMemo(() => {
    const m = new Map<string, string>()
    for (const f of flags) m.set(f.id, f.label)
    return m
  }, [flags])

  // Layout once on structure (flag IDs + edges never change shape).
  const structureKey = useMemo(() => flags.map((f) => f.id).join('|'), [flags])
  const initial = useMemo(() => {
    const rawNodes: Node[] = flags.map((f) => ({
      id: f.id,
      type: 'flag',
      position: { x: 0, y: 0 },
      data: {
        label: f.label, zoneShort: f.zone_short, status: statusOf(f),
        title: '', kind: f.step_kind,
      } satisfies FlagNodeData,
    }))
    const rawEdges: Edge[] = []
    for (const f of flags) {
      for (const p of f.prereqs) {
        rawEdges.push({
          id: `${p}->${f.id}`,
          source: p,
          target: f.id,
          type: 'smoothstep',
          style: { stroke: '#4b5563', strokeWidth: 1.5, opacity: 0.7 },
          markerEnd: { type: MarkerType.ArrowClosed, color: '#4b5563', width: 13, height: 13 },
        })
      }
      // Any-of members feed their anchor with a dashed amber edge — completing
      // ANY one satisfies the anchor (distinct from the solid AND-prereq edges).
      if (f.group) {
        rawEdges.push({
          id: `grp:${f.id}->${f.group}`,
          source: f.id,
          target: f.group,
          type: 'smoothstep',
          style: { stroke: '#fb923c', strokeWidth: 1.5, opacity: 0.6, strokeDasharray: '4 3' },
          markerEnd: { type: MarkerType.ArrowClosed, color: '#fb923c', width: 13, height: 13 },
        })
      }
    }
    return { nodes: layoutGraph(rawNodes, rawEdges), edges: rawEdges }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [structureKey])

  const [nodes, setNodes, onNodesChange] = useNodesState<Node>(initial.nodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>(initial.edges)
  const [focusedId, setFocusedId] = useState<string | null>(null)

  // Re-derive node colour + dim/highlight whenever state or focus changes,
  // without touching positions (so dragging/zoom survive a toggle).
  useEffect(() => {
    let inFocus: Set<string> | null = null
    if (focusedId) {
      const n = new Set<string>([focusedId])
      for (const e of edges) {
        if (e.source === focusedId) n.add(e.target)
        if (e.target === focusedId) n.add(e.source)
      }
      inFocus = n
    }
    const byId = new Map(flags.map((f) => [f.id, f]))
    setNodes((current) =>
      current.map((node) => {
        const f = byId.get(node.id)
        if (!f) return node
        const status = statusOf(f)
        const missing = (f.missing ?? []).map((id) => labelById.get(id) ?? id)
        const title = f.locked && missing.length > 0 ? `${f.detail}\n\nNeeds: ${missing.join(', ')}` : f.detail
        return {
          ...node,
          data: {
            ...(node.data as FlagNodeData),
            status, title,
            dimmed: inFocus ? !inFocus.has(node.id) : false,
            highlighted: focusedId === node.id,
          },
        }
      }),
    )
    setEdges((current) =>
      current.map((e) => {
        const dimmed = inFocus ? !(inFocus.has(e.source) && inFocus.has(e.target)) : false
        return { ...e, style: { ...(e.style ?? {}), opacity: dimmed ? 0.06 : 0.7 } }
      }),
    )
  }, [flags, focusedId, labelById, setNodes, setEdges, edges])

  const handleNodeClick: NodeMouseHandler = useCallback((_, node) => {
    setFocusedId((cur) => (cur === node.id ? null : node.id))
  }, [])
  const handlePaneClick = useCallback(() => setFocusedId(null), [])

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 px-4 py-2 shrink-0">
        <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          Click a node to highlight its prerequisites and dependents; click empty space to reset. Drag to
          reorganise, scroll to zoom.
        </p>
        <div className="ml-auto flex items-center gap-2 text-[11px]">
          {(Object.entries(STATUS_COLORS) as Array<[Status, typeof STATUS_COLORS[Status]]>).map(([k, p]) => (
            <span
              key={k}
              className="inline-flex items-center gap-1 rounded px-1.5 py-0.5"
              style={{ color: p.border, border: `1px solid ${p.border}`, backgroundColor: p.bg }}
            >
              <span style={{ display: 'inline-block', width: 8, height: 8, backgroundColor: p.border, borderRadius: 2 }} />
              {p.label}
            </span>
          ))}
        </div>
      </div>
      <div className="flex-1" style={{ backgroundColor: '#0b0f17' }}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onNodeClick={handleNodeClick}
          onPaneClick={handlePaneClick}
          nodeTypes={nodeTypes}
          fitView
          fitViewOptions={{ padding: 0.15 }}
          minZoom={0.1}
          maxZoom={2.5}
          proOptions={{ hideAttribution: true }}
        >
          <Background color="#1f2937" gap={24} />
        </ReactFlow>
      </div>
    </div>
  )
}
