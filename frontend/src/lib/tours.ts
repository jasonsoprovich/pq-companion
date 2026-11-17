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
]

export function tourById(id: string): TourDef | undefined {
  return TOURS.find((t) => t.id === id)
}

// toursForPath lists tours whose route matches pathname, for the help
// drawer's "Take the tour" link.
export function toursForPath(pathname: string): TourDef[] {
  return TOURS.filter((t) => t.route.split('?')[0] === pathname)
}
