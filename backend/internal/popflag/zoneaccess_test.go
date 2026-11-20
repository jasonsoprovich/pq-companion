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
