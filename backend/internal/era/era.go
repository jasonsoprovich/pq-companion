// Package era centralizes Project Quarm's expansion-era gating. The server
// is currently in the pre-Planes-of-Power era (level cap 60, PoP content
// locked); PoP launches later in 2026. Every level cap or era-conditional
// branch in the app should derive from this package so the whole app swaps
// over together when Preferences.PoPEnabled flips — via the Developer-tab
// preview toggle today, and via the default at launch.
//
// Phase B/C of the PoP rollout (content additions, data refresh, default
// flip) builds on this package; see the era-gated call sites for what each
// flag value controls.
package era

const (
	// PrePoPMaxLevel is the level cap while Planes of Power is locked.
	PrePoPMaxLevel = 60
	// PoPMaxLevel is the level cap once Planes of Power is live.
	PoPMaxLevel = 65
)

// MaxLevel returns the server level cap for the given era state. popActive
// is Preferences.PoPEnabled — false until the expansion launches.
func MaxLevel(popActive bool) int {
	if popActive {
		return PoPMaxLevel
	}
	return PrePoPMaxLevel
}
