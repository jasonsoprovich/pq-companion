package trigger

import (
	"regexp"
	"strings"
	"testing"
)

// Every transformed pack pattern must still compile, and any buff trigger
// with a cast-on-other branch must end up with target capture wired.
func TestAllPacks_BuffTargetCaptureCompilesAndWires(t *testing.T) {
	wired := 0
	for _, p := range AllPacks() {
		for _, tr := range p.Triggers {
			if _, err := regexp.Compile(tr.Pattern); err != nil {
				t.Fatalf("%s/%s pattern won't compile: %v", p.PackName, tr.Name, err)
			}
			if tr.WornOffPattern != "" {
				if _, err := regexp.Compile(tr.WornOffPattern); err != nil {
					t.Fatalf("%s/%s worn-off won't compile: %v", p.PackName, tr.Name, err)
				}
			}
			if tr.TimerType == TimerTypeBuff && strings.Contains(tr.Pattern, "|"+playerNameClass) {
				t.Errorf("%s/%s still has an unwrapped cast-on-other branch", p.PackName, tr.Name)
			}
			if tr.TimerTargetCapture == "target" {
				wired++
				if !strings.Contains(tr.Pattern, "(?P<target>") {
					t.Errorf("%s/%s sets target capture but pattern has no target group", p.PackName, tr.Name)
				}
			}
		}
	}
	if wired == 0 {
		t.Fatal("expected at least some buff triggers to gain target capture")
	}
	t.Logf("buff triggers wired for target capture: %d", wired)
}
