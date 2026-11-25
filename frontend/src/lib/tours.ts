// Guided tour definitions. A tour points at elements tagged with
// data-tour="<anchor>" and walks the user through them with a spotlight
// and a popover (components/guide/Tour.tsx).
//
// Starting a tour navigates to `route` first, then waits for the `ready`
// anchor to mount before showing step one. A tour that can't find its
// ready anchor isn't started and isn't marked seen. Steps whose anchor is
// missing once the tour is running are skipped.

export interface TourStep {
  // data-tour value of the element to spotlight.
  anchor: string
  title: string
  body: string
}

export interface TourDef {
  id: string
  title: string
  // One line for the Help → Tours list.
  description: string
  // Path (plus optional query, e.g. '/overlays?panel=timers') to open
  // before the tour starts.
  route: string
  // data-tour anchor that must be mounted before step one shows.
  ready: string
  steps: TourStep[]
}

export const TOURS: TourDef[] = [
  {
    id: 'help',
    title: 'Help Center',
    description: 'Where to find answers, guides and tours inside the app.',
    route: '/help',
    ready: 'help-tabs',
    steps: [
      {
        anchor: 'help-tabs',
        title: 'Three ways in',
        body:
          'Quick Start gets a new install running. Reference covers every ' +
          'feature in detail. Tours replays any guided walkthrough.',
      },
      {
        anchor: 'help-search',
        title: 'Search everything',
        body:
          'Type a feature, setting or question. Results search article ' +
          'titles and their full text.',
      },
      {
        anchor: 'help-drawer-button',
        title: 'Help on any page',
        body:
          'The ? button opens the help for whatever page you are on, ' +
          'without leaving it.',
      },
    ],
  },
  {
    id: 'triggers',
    title: 'Triggers',
    description: 'Alerts, timers and trigger packs: where everything lives.',
    route: '/triggers',
    ready: 'triggers-tabs',
    steps: [
      {
        anchor: 'triggers-tabs',
        title: 'Triggers and timers',
        body:
          'A trigger watches your log for a line and then speaks, shows ' +
          'text, plays a sound or starts a timer. Your triggers are listed ' +
          'here by category.',
      },
      {
        anchor: 'triggers-new',
        title: 'Make your own',
        body:
          'New Trigger offers templates: a text alert, spell timer, boss ' +
          'emote warning, tell alert or countdown, each in a few clicks. ' +
          'The arrow jumps straight to a spell timer or a blank trigger.',
      },
      {
        anchor: 'triggers-tab-packs',
        title: 'Install a pack',
        body:
          'The quickest start: packs bundle ready-made triggers for your ' +
          'class or for raid targets. You can also import GINA or ' +
          'EQLogParser triggers here. A dot means a pack has updates.',
      },
      {
        anchor: 'triggers-tab-tester',
        title: 'Test before raid night',
        body:
          'Paste log lines into the Tester to see which triggers match ' +
          'and what they would do, without being in game.',
      },
      {
        anchor: 'triggers-tab-history',
        title: 'What fired',
        body: 'History lists recent trigger fires, handy for checking an alert went off.',
      },
    ],
  },
  {
    id: 'overlays',
    title: 'Overlays',
    description: 'The overlay library, arranging windows, and making windows of your own.',
    route: '/overlays?tab=library',
    ready: 'overlays-library',
    steps: [
      {
        anchor: 'overlays-library',
        title: 'Every overlay, one card each',
        body:
          'Click a card for its settings. Pop out opens it over the game; ' +
          'the switch puts it on the Dashboard tab instead.',
      },
      {
        anchor: 'overlays-views',
        title: 'Arrange and layouts',
        body:
          'Arrange shows your screens so you can drag windows into place. ' +
          'Layouts (the chips beside it) save a whole setup to switch between.',
      },
      {
        anchor: 'overlays-your-windows',
        title: 'Make your own windows',
        body:
          'The dashed + tiles make windows of your own: a timer window for ' +
          'named raid or boss timers, a custom overlay built from widgets and ' +
          'your own log rules, or a chat window that filters chat into one ' +
          'list or tabs. The new window gets a card here; click it to set it up.',
      },
      {
        anchor: 'overlays-actions',
        title: 'Settings for all of them',
        body:
          'Overlay settings covers every overlay at once (opacity, fade, ' +
          'lock modes). The ⋯ menu runs Overlay Setup again, closes all ' +
          'popouts or resets window positions.',
      },
    ],
  },
  {
    id: 'timers',
    title: 'Timer settings',
    description: 'Which spells get timers, when they show, and how they look.',
    route: '/overlays?panel=timers',
    ready: 'timers-tracking',
    steps: [
      {
        anchor: 'timers-tracking',
        title: 'What gets a timer',
        body:
          'Tracking mode picks every recognised spell (Auto) or only ones ' +
          'from your triggers. Scope decides whose casts count: just yours, ' +
          'or anyone you see land a spell.',
      },
      {
        anchor: 'timers-thresholds',
        title: 'Keep long buffs out of the way',
        body:
          'A threshold hides a timer until it has less than that many ' +
          'seconds left, so hour-long buffs only show near the end.',
      },
      {
        anchor: 'timers-seconds',
        title: 'Look and feel',
        body:
          'These settings are shared by every timer overlay (buffs, ' +
          'detrimentals, custom and filtered). Help → Reference → How timers ' +
          'work explains the whole pipeline.',
      },
    ],
  },
  {
    id: 'chchain',
    title: 'CH Chain',
    description: 'Track a Complete Heal rotation from chat calls.',
    route: '/overlays?panel=chChain',
    ready: 'chchain-enable',
    steps: [
      {
        anchor: 'chchain-enable',
        title: 'Follow the chain',
        body:
          'With tracking on, CH calls in raid, group, guild or your chosen ' +
          'channels become one bar per position, counting down each 10 s cast.',
      },
      {
        anchor: 'chchain-miss',
        title: 'Spot a missed heal',
        body:
          'Turns a bar red when that healer is never seen starting a cast ' +
          'in time (a fizzle, interrupt or skipped cast), so someone can ' +
          'cover.',
      },
      {
        anchor: 'chchain-secondary',
        title: 'A second chain',
        body: 'Run a ramp or split-heal chain alongside the main one, in its own section.',
      },
    ],
  },
  {
    id: 'settings',
    title: 'Settings',
    description: 'Find any setting fast and check your setup is healthy.',
    route: '/settings?tab=general',
    ready: 'settings-tabs',
    steps: [
      {
        anchor: 'settings-search',
        title: 'Search settings',
        body:
          'Type what you want to change (voice, zoom, backups) to jump ' +
          'straight to it. Ctrl+K finds settings from anywhere too.',
      },
      {
        anchor: 'settings-health',
        title: 'Setup health',
        body:
          'Green means working. Anything amber or red has a Fix button ' +
          'that takes you to the setting that sorts it.',
      },
      {
        anchor: 'settings-tabs',
        title: 'Everything else',
        body:
          'Settings are grouped by area. Overlay and timer settings live ' +
          'in the Overlays hub instead.',
      },
      {
        anchor: 'sidebar-edit',
        title: 'Make the menu yours',
        body:
          'The pencil lets you reorder, favorite or hide menu items. You ' +
          'can also right-click any item.',
      },
    ],
  },
]

export function tourById(id: string): TourDef | undefined {
  return TOURS.find((t) => t.id === id)
}

// toursForPath lists tours whose route matches pathname, for the help
// drawer's "Take the tour" link.
export function toursForPath(pathname: string): TourDef[] {
  return TOURS.filter((t) => t.route.split('?')[0] === pathname)
}
