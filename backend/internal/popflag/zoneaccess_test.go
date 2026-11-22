package popflag

import "testing"

func TestZoneAccessIDsExist(t *testing.T) {
	for zone, ids := range ZoneAccess() {
		if len(ids) == 0 {
			t.Errorf("%s: empty access list", zone)
		}
		for _, id := range ids {
			if _, ok := ByID(id); !ok {
				t.Errorf("%s: unknown flag id %q", zone, id)
			}
		}
	}
}

// Level access is an alternative to a zone's flags, so every level-access
// zone must also be a flag-gated zone the page knows the steps for.
func TestZoneLevelAccessZonesAreFlagGated(t *testing.T) {
	access := ZoneAccess()
	for zone, lvl := range ZoneLevelAccess() {
		if _, ok := access[zone]; !ok {
			t.Errorf("%s has level access but no flag steps", zone)
		}
		if lvl < 46 || lvl > 65 {
			t.Errorf("%s: level %d out of range", zone, lvl)
		}
	}
}
