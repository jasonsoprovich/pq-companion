package respawn

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

func TestPlaceSpawn(t *testing.T) {
	now := time.Now()
	infos := []db.RespawnInfo{
		{SpawnID: 1, X: 0, Y: 0},
		{SpawnID: 2, X: 500, Y: 0},
		{SpawnID: 2, X: 500, Y: 0}, // same point, another variant row
		{SpawnID: 3, X: 0, Y: 900},
	}
	tests := []struct {
		name  string
		setup func(e *Engine)
		want  int
	}{
		{"no position, several points", func(e *Engine) {}, 0},
		{"player position", func(e *Engine) { e.SetPipePlayerPos(450, 30) }, 2},
		{"target position beats player", func(e *Engine) {
			e.SetPipePlayerPos(450, 30)
			e.SetPipeTargetLocAt("A Gnoll", 10, 850, now.Add(-2*time.Second))
		}, 3},
		{"stale target position", func(e *Engine) {
			e.SetPipePlayerPos(450, 30)
			e.SetPipeTargetLocAt("a gnoll", 10, 850, now.Add(-time.Minute))
		}, 2},
		{"other target", func(e *Engine) {
			e.SetPipePlayerPos(450, 30)
			e.SetPipeTargetLocAt("an orc", 10, 850, now)
		}, 2},
		{"taken point is skipped", func(e *Engine) {
			e.SetPipePlayerPos(450, 30)
			e.timers["x"] = &RespawnTimer{ID: "x", Zone: "z", SpawnID: 2, RespawnAt: now.Add(time.Minute)}
		}, 1},
		{"only free point needs no position", func(e *Engine) {
			e.timers["x"] = &RespawnTimer{ID: "x", Zone: "z", SpawnID: 2, RespawnAt: now.Add(time.Minute)}
			e.timers["y"] = &RespawnTimer{ID: "y", Zone: "z", SpawnID: 3, RespawnAt: now.Add(time.Minute)}
		}, 1},
		{"popped timer frees its point", func(e *Engine) {
			e.SetPipePlayerPos(450, 30)
			e.timers["x"] = &RespawnTimer{ID: "x", Zone: "z", SpawnID: 2, RespawnAt: now.Add(-time.Second)}
		}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEngine(nil, nil)
			tc.setup(e)
			e.mu.Lock()
			got := e.placeSpawnLocked("z", "", "a gnoll", infos, now)
			e.mu.Unlock()
			if got != tc.want {
				t.Fatalf("spawn = %d, want %d", got, tc.want)
			}
		})
	}
}
