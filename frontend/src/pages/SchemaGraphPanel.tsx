import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ReactFlow,
  Background,
  Controls,
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
import { Search, X, RefreshCw } from 'lucide-react'
import { SCHEMA_TABLES, SCHEMA_EDGES, CATEGORY_COLORS, type SchemaCategory } from '../lib/schemaGraph'

// ── Layout ───────────────────────────────────────────────────────────────────
//
// React Flow has no built-in auto-layout, so we run dagre over the graph
// once and feed it positions. nodesep/ranksep control spacing; LR
// (left-to-right) reads better for ER-style graphs than TB.

const NODE_WIDTH = 220
const HEADER_HEIGHT = 32

function nodeHeightFor(columnCount: number): number {
  // Header + 18px per visible column row + a little padding.
  return HEADER_HEIGHT + columnCount * 18 + 8
}

function layoutGraph(nodes: Node[], edges: Edge[]): Node[] {
  const g = new dagre.graphlib.Graph()
  g.setGraph({ rankdir: 'LR', nodesep: 40, ranksep: 90, marginx: 20, marginy: 20 })
  g.setDefaultEdgeLabel(() => ({}))

  for (const n of nodes) {
    const h = (n.data as { columns?: unknown[] }).columns?.length ?? 0
    g.setNode(n.id, { width: NODE_WIDTH, height: nodeHeightFor(h) })
  }
  for (const e of edges) g.setEdge(e.source, e.target)

  dagre.layout(g)

  return nodes.map((n) => {
    const pos = g.node(n.id)
    const h = (n.data as { columns?: unknown[] }).columns?.length ?? 0
    return {
      ...n,
      position: { x: pos.x - NODE_WIDTH / 2, y: pos.y - nodeHeightFor(h) / 2 },
    }
  })
}

// ── Node renderer ────────────────────────────────────────────────────────────
//
// One card per table. Header is the table name in the category colour; body
// is a vertically stacked column list (name + type, plus a PK / FK chip).
// Width is fixed so dagre's layout numbers stay accurate.

interface TableNodeData {
  name: string
  category: SchemaCategory
  columns: Array<{ name: string; type: string; pk?: boolean; fk?: boolean }>
  dimmed?: boolean
  highlighted?: boolean
  [key: string]: unknown
}

// Edges carry no explicit handle ids, so the node must expose the default
// (unnamed) source/target handles or React Flow can't attach any edge (error
// #008). Hidden because the schema graph isn't interactively editable — the
// handles exist purely as edge anchor points. LR layout → target on the left,
// source on the right.
const hiddenHandleStyle: React.CSSProperties = {
  opacity: 0,
  width: 1,
  height: 1,
  minWidth: 0,
  minHeight: 0,
  border: 'none',
  background: 'transparent',
  pointerEvents: 'none',
}

function TableNode({ data }: { data: TableNodeData }): React.ReactElement {
  const palette = CATEGORY_COLORS[data.category]
  return (
    <div
      style={{
        width: NODE_WIDTH,
        backgroundColor: palette.bg,
        border: `1px solid ${data.highlighted ? '#fff' : palette.border}`,
        borderRadius: 4,
        opacity: data.dimmed ? 0.18 : 1,
        transition: 'opacity 0.15s ease, border-color 0.15s ease',
        boxShadow: data.highlighted ? `0 0 0 2px ${palette.border}` : 'none',
        fontSize: 11,
        fontFamily: 'monospace',
        overflow: 'hidden',
      }}
    >
      <Handle type="target" position={Position.Left} style={hiddenHandleStyle} isConnectable={false} />
      <Handle type="source" position={Position.Right} style={hiddenHandleStyle} isConnectable={false} />
      <div
        style={{
          height: HEADER_HEIGHT,
          display: 'flex',
          alignItems: 'center',
          padding: '0 8px',
          backgroundColor: palette.border + '33',
          color: palette.border,
          fontWeight: 600,
          borderBottom: `1px solid ${palette.border}55`,
        }}
      >
        {data.name}
      </div>
      <div>
        {data.columns.map((c) => (
          <div
            key={c.name}
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              padding: '1px 8px',
              color: '#e5e7eb',
              gap: 4,
            }}
          >
            <span
              style={{
                color: c.pk ? '#fbbf24' : c.fk ? '#93c5fd' : '#e5e7eb',
                fontWeight: c.pk ? 600 : 400,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {c.name}
            </span>
            <span style={{ color: '#9ca3af', flexShrink: 0 }}>
              {c.type}
              {c.pk ? ' PK' : c.fk ? ' FK' : ''}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

const nodeTypes = { table: TableNode }

// ── Build initial graph data from the schemaGraph source ─────────────────────

function buildInitial(): { nodes: Node[]; edges: Edge[] } {
  const tableSet = new Set(SCHEMA_TABLES.map((t) => t.name))
  const rawNodes: Node[] = SCHEMA_TABLES.map((t) => ({
    id: t.name,
    type: 'table',
    position: { x: 0, y: 0 },
    data: {
      name: t.name,
      category: t.category,
      columns: t.columns,
    } satisfies TableNodeData,
  }))

  const rawEdges: Edge[] = SCHEMA_EDGES
    // Drop any edge whose endpoint isn't in our curated table list (the
    // graph would render disconnected stubs otherwise).
    .filter((e) => tableSet.has(e.from) && tableSet.has(e.to))
    .map((e, i) => {
      const palette = CATEGORY_COLORS[
        (SCHEMA_TABLES.find((t) => t.name === e.from)?.category ?? 'items') as SchemaCategory
      ]
      return {
        id: `e${i}`,
        source: e.from,
        target: e.to,
        // ?-shaped routing reads more naturally than straight lines on
        // an LR-layout ER graph.
        type: 'smoothstep',
        label: `${e.fromCol} → ${e.toCol}`,
        labelStyle: { fontSize: 10, fill: '#9ca3af', fontFamily: 'monospace' },
        labelBgStyle: { fill: '#111827', fillOpacity: 0.85 },
        labelBgPadding: [4, 2] as [number, number],
        labelBgBorderRadius: 2,
        style: { stroke: palette.border, strokeWidth: 1.5, opacity: 0.7 },
        markerEnd: { type: MarkerType.ArrowClosed, color: palette.border, width: 14, height: 14 },
        data: { note: e.note },
      } as Edge
    })

  const laid = layoutGraph(rawNodes, rawEdges)
  return { nodes: laid, edges: rawEdges }
}

// ── Panel ────────────────────────────────────────────────────────────────────

export default function SchemaGraphPanel(): React.ReactElement {
  const initial = useMemo(() => buildInitial(), [])
  const [nodes, setNodes, onNodesChange] = useNodesState<Node>(initial.nodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>(initial.edges)
  const [focusedId, setFocusedId] = useState<string | null>(null)
  const [search, setSearch] = useState('')

  // Re-derive dimmed/highlighted state on the nodes whenever the focus or
  // search query changes. We mutate node.data, not the node graph itself,
  // so React Flow keeps its position / drag state intact.
  useEffect(() => {
    const lowerSearch = search.trim().toLowerCase()

    // Compute the "in focus" set: focused node + everything it touches.
    let inFocus: Set<string> | null = null
    if (focusedId) {
      const neighbours = new Set<string>([focusedId])
      for (const e of edges) {
        if (e.source === focusedId) neighbours.add(e.target)
        if (e.target === focusedId) neighbours.add(e.source)
      }
      inFocus = neighbours
    } else if (lowerSearch) {
      const matches = SCHEMA_TABLES.filter((t) => t.name.toLowerCase().includes(lowerSearch))
      const neighbours = new Set<string>(matches.map((t) => t.name))
      for (const e of edges) {
        if (neighbours.has(e.source)) neighbours.add(e.target)
        if (neighbours.has(e.target)) neighbours.add(e.source)
      }
      inFocus = neighbours
    }

    setNodes((current) =>
      current.map((n) => {
        const dimmed = inFocus ? !inFocus.has(n.id) : false
        const highlighted = focusedId === n.id || (lowerSearch !== '' && n.id.toLowerCase().includes(lowerSearch))
        return {
          ...n,
          data: { ...(n.data as TableNodeData), dimmed, highlighted },
        }
      }),
    )
    setEdges((current) =>
      current.map((e) => {
        const dimmed = inFocus ? !(inFocus.has(e.source) && inFocus.has(e.target)) : false
        return {
          ...e,
          style: { ...(e.style ?? {}), opacity: dimmed ? 0.08 : 0.7 },
          labelStyle: { ...(e.labelStyle ?? {}), opacity: dimmed ? 0.1 : 1 },
        }
      }),
    )
  }, [focusedId, search, setNodes, setEdges, edges])

  const handleNodeClick: NodeMouseHandler = useCallback((_, node) => {
    setFocusedId((cur) => (cur === node.id ? null : node.id))
  }, [])

  const handlePaneClick = useCallback(() => {
    setFocusedId(null)
  }, [])

  const resetLayout = useCallback(() => {
    const fresh = buildInitial()
    setNodes(fresh.nodes)
    setEdges(fresh.edges)
    setFocusedId(null)
    setSearch('')
  }, [setNodes, setEdges])

  // Category legend — rendered as little chips so users know what each
  // border colour means without hunting through the data file.
  const categories = Object.entries(CATEGORY_COLORS) as Array<[SchemaCategory, { bg: string; border: string; label: string }]>

  return (
    <section
      className="rounded-lg p-4"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
      }}
    >
      <div className="mb-3 flex flex-wrap items-center gap-2">
        {/* Search */}
        <div
          className="flex items-center gap-2 rounded px-2 py-1"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            minWidth: 220,
          }}
        >
          <Search size={12} style={{ color: 'var(--color-muted)' }} />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Filter tables…"
            spellCheck={false}
            style={{
              background: 'transparent',
              border: 'none',
              outline: 'none',
              color: 'var(--color-foreground)',
              fontSize: 12,
              width: '100%',
            }}
          />
          {search && (
            <button
              type="button"
              onClick={() => setSearch('')}
              title="Clear filter"
              style={{ background: 'transparent', border: 'none', cursor: 'pointer', color: 'var(--color-muted)', padding: 0 }}
            >
              <X size={12} />
            </button>
          )}
        </div>

        <button
          type="button"
          onClick={resetLayout}
          className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
            cursor: 'pointer',
          }}
        >
          <RefreshCw size={12} />
          Reset layout
        </button>

        <div className="ml-auto flex flex-wrap items-center gap-2 text-[11px]">
          {categories.map(([key, palette]) => (
            <span
              key={key}
              className="inline-flex items-center gap-1 rounded px-1.5 py-0.5"
              style={{
                color: palette.border,
                border: `1px solid ${palette.border}`,
                backgroundColor: palette.bg,
              }}
            >
              <span
                style={{
                  display: 'inline-block',
                  width: 8,
                  height: 8,
                  backgroundColor: palette.border,
                  borderRadius: 2,
                }}
              />
              {palette.label}
            </span>
          ))}
        </div>
      </div>

      <p className="mb-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        Click a table to highlight only its neighbours; click empty space to
        reset. Drag tables to reorganise; scroll to zoom. Edge labels show
        the join columns (<code>source → target</code>).
      </p>

      <div
        style={{
          height: 640,
          width: '100%',
          borderRadius: 6,
          border: '1px solid var(--color-border)',
          backgroundColor: '#0b0f17',
        }}
      >
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
          minZoom={0.15}
          maxZoom={2.5}
          proOptions={{ hideAttribution: true }}
        >
          <Background color="#1f2937" gap={24} />
          <Controls
            position="bottom-right"
            showInteractive={false}
            style={{ background: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
          />
        </ReactFlow>
      </div>
    </section>
  )
}
