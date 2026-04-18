// HPS tracking is disabled until healing events are present in EQ log files.
// Set VITE_DEV_HPS=true in .env.local to re-enable during development.
export const DEV_HPS = import.meta.env.VITE_DEV_HPS === 'true'
