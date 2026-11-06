import React from 'react'
import type { LiveMapOverlayPrefs } from '../../hooks/useLiveMapOverlayPrefs'
import type { Map3DLabels } from '../../hooks/useMap3DPrefs'
import type { AggroScope } from './spawnOverlays'
import type { Camera3D } from './ZoneMap3D'

const RINGS_NEXT: Record<AggroScope, AggroScope> = { off: 'near', near: 'all', all: 'off' }
const RINGS_NAME: Record<AggroScope, string> = { off: 'Off', near: 'Near', all: 'All' }
const CAMERA_NEXT: Record<Camera3D, Camera3D> = { free: 'top', top: 'chase', chase: 'free' }
const CAMERA_NAME: Record<Camera3D, string> = { free: 'Free', top: 'Top', chase: 'Chase' }
const LABELS_NEXT: Record<Map3DLabels, Map3DLabels> = { off: 'group', group: 'all', all: 'off' }
const LABELS_NAME: Record<Map3DLabels, string> = { off: 'Off', group: 'Group', all: 'All' }

interface Props {
  prefs: LiveMapOverlayPrefs
  onChange: (patch: Partial<LiveMapOverlayPrefs>) => void
  // groupDefault is the Settings default the Group toggle starts from.
  groupDefault: boolean
  // full adds the Near distance slider, for Settings, where there's room.
  full?: boolean
}

// LiveMapOverlayControls is the map overlay's toggles as a row of small
// chips: the overlay's hover bar, and the same row in Settings.
export default function LiveMapOverlayControls({ prefs, onChange, groupDefault, full }: Props): React.ReactElement {
  const groupOn = prefs.group ?? groupDefault
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 4 }}>
      <Chip on={!prefs.view3D} onClick={() => onChange({ view3D: false })} title="Flat outline map">2D</Chip>
      <Chip
        on={prefs.view3D}
        onClick={() => onChange({ view3D: true })}
        title="The zone in 3D from your EverQuest files, following you"
      >
        3D
      </Chip>
      {prefs.view3D ? (
        <Chip
          on={prefs.camera3D !== 'free'}
          onClick={() => onChange({ camera3D: CAMERA_NEXT[prefs.camera3D] })}
          title="Camera: free to orbit, looking straight down, or riding behind you facing your way"
        >
          Camera: {CAMERA_NAME[prefs.camera3D]}
        </Chip>
      ) : (
        <Chip
          on={prefs.headingUp}
          onClick={() => onChange({ headingUp: !prefs.headingUp })}
          title="Turn the map so the way you face is up (a compass shows north)"
        >
          Heading up
        </Chip>
      )}
      <Sep />
      <Chip on={groupOn} onClick={() => onChange({ group: !groupOn })} title="Your groupmates in this zone">
        Group
      </Chip>
      <Chip on={prefs.raid} onClick={() => onChange({ raid: !prefs.raid })} title="Everyone else in your raid in this zone">
        Raid
      </Chip>
      {prefs.raid && (
        <Chip
          on
          onClick={() => onChange({ raidColor: prefs.raidColor === 'group' ? 'class' : 'group' })}
          title="Colour raiders by raid group or by class"
        >
          {prefs.raidColor === 'group' ? 'By group' : 'By class'}
        </Chip>
      )}
      <Chip
        on={prefs.target}
        onClick={() => onChange({ target: !prefs.target })}
        title="Your target where it stands, a line to it, and its aggro ring (within 250 units)"
      >
        Target
      </Chip>
      <Sep />
      <Chip
        on={prefs.aggro !== 'off'}
        onClick={() => onChange({ aggro: RINGS_NEXT[prefs.aggro] })}
        title="Aggro rings: off, near you, or every spawn point's"
      >
        Rings: {RINGS_NAME[prefs.aggro]}
      </Chip>
      <Chip on={prefs.dots} onClick={() => onChange({ dots: !prefs.dots })} title="Spawn points, coloured by respawn timer">
        Spawns
      </Chip>
      {prefs.dots && (
        <Chip
          on={prefs.hideKilled}
          onClick={() => onChange({ hideKilled: !prefs.hideKilled })}
          title="Leave out spawn points whose mob is dead and not back within 2 minutes"
        >
          Hide killed
        </Chip>
      )}
      <Chip
        on={prefs.labels !== 'off'}
        onClick={() => onChange({ labels: LABELS_NEXT[prefs.labels] })}
        title="Whose names show: nobody, you and your group, or everyone close enough"
      >
        Labels: {LABELS_NAME[prefs.labels]}
      </Chip>
      {full && prefs.aggro === 'near' && (
        <label style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11, color: 'var(--color-muted)' }}>
          Rings within
          <input
            type="range"
            min={100}
            max={1500}
            step={50}
            value={prefs.aggroNear}
            onChange={(e) => onChange({ aggroNear: Number(e.target.value) })}
          />
          <span style={{ fontVariantNumeric: 'tabular-nums' }}>{prefs.aggroNear}</span>
        </label>
      )}
    </div>
  )
}

function Chip({ on, onClick, title, children }: {
  on: boolean
  onClick: () => void
  title: string
  children: React.ReactNode
}): React.ReactElement {
  return (
    <button
      onClick={onClick}
      title={title}
      style={{
        fontSize: 10,
        lineHeight: 1,
        padding: '3px 6px',
        borderRadius: 3,
        border: `1px solid ${on ? 'rgba(201,168,76,0.7)' : 'rgba(255,255,255,0.12)'}`,
        backgroundColor: on ? 'rgba(201,168,76,0.15)' : 'rgba(0,0,0,0.35)',
        color: on ? '#c9a84c' : 'rgba(255,255,255,0.55)',
        cursor: 'pointer',
        whiteSpace: 'nowrap',
      }}
    >
      {children}
    </button>
  )
}

function Sep(): React.ReactElement {
  return <span style={{ width: 1, alignSelf: 'stretch', backgroundColor: 'rgba(255,255,255,0.1)' }} />
}
