package main

import (
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"time"
)

// Shared by shipping main and physical-file integration tests; never calls
// main or discovers Windows IPC. Raw zone boundaries are processed before
// the next physical line, not in the Tailer's delayed parsed-event batch.
func wireEncounterTimers(tailer *logparser.Tailer, engine *trigger.Engine, guard *consumerGuard) {
	tailer.SetContextHandlers(engine.SourceChanged, func(ts time.Time, msg string, g uint64) {
		guard.run("triggerEngine", func() { engine.HandleSource(ts, msg, g) })
	})
}
