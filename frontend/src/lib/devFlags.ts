// HPS tracking is disabled until healing events are present in EQ log files.
// Set VITE_DEV_HPS=true in .env.local to re-enable during development.
export const DEV_HPS = import.meta.env.VITE_DEV_HPS === 'true'

// The Skill Tracker is disabled by default: skills can only be learned from
// "You have become better at X!" log lines (deltas), and no full skill
// snapshot is available from any source — the Quarmy/Zeal export carries no
// skills and the ZealPipes LabelType enum has no skill entries. A character
// already at cap (or whose old logs were purged) can therefore never populate
// the page, which feels broken for most users. Hidden behind this flag until a
// skill-snapshot data source exists. Set VITE_DEV_SKILLS=true in .env.local to
// re-enable during development. See LIMITATIONS.md §7.2.
export const DEV_SKILLS = import.meta.env.VITE_DEV_SKILLS === 'true'
