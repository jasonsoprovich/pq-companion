package trigger

import (
	"strings"
	"sync"
	"time"
)

// groupMemberToken is the pattern token (case-insensitive) that matches one
// player name and gates the trigger on that name being in the active
// character's group. normalizePattern expands it to a capture group of the
// same name, so alert text can also say {groupmember}.
const groupMemberToken = "groupmember"

// groupMemberCapture is the regex every {groupmember} expands to. Player
// names are a single word of letters.
const groupMemberCapture = `(?P<groupmember>[A-Za-z]+)`

const (
	// rosterPipeLive is how recently any Zeal frame must have arrived for
	// the roster to count as known. Zeal resends labels every pipe_delay
	// (100 ms default), so a few seconds of silence means the pipe is gone.
	rosterPipeLive = 5 * time.Second
	// rosterMemberTTL is how long a name stays a member after its last
	// group frame. Zeal lists only members whose entity is loaded in the
	// player's zone, and a member who just died can drop out of the frame
	// before the "has been slain" line is matched — so a short memory
	// keeps that line counted. It also means a member who left the group
	// counts for this long afterwards.
	rosterMemberTTL = 30 * time.Second
)

// GroupRoster tracks each character's current group from Zeal group frames
// (MsgGroup). Zeal sends those only while the character is in a group, so a
// live pipe with no recent group frame means the character is solo.
// Safe for concurrent use.
type GroupRoster struct {
	mu    sync.Mutex
	now   func() time.Time
	chars map[string]*rosterState // keyed by lower-cased character name
}

type rosterState struct {
	pipeAt  time.Time
	members map[string]time.Time // lower-cased member name → last seen
}

// NewGroupRoster returns an empty roster.
func NewGroupRoster() *GroupRoster {
	return &GroupRoster{now: time.Now, chars: map[string]*rosterState{}}
}

func (r *GroupRoster) stateLocked(character string) *rosterState {
	key := strings.ToLower(character)
	s := r.chars[key]
	if s == nil {
		s = &rosterState{members: map[string]time.Time{}}
		r.chars[key] = s
	}
	return s
}

// ObservePipe records that a Zeal frame of any type arrived for character.
// Call it for every envelope; it's what tells "solo" apart from "no Zeal".
func (r *GroupRoster) ObservePipe(character string) {
	if character == "" {
		return
	}
	r.mu.Lock()
	r.stateLocked(character).pipeAt = r.now()
	r.mu.Unlock()
}

// ObserveGroup records the member names in one of character's group frames.
// The character's own name is skipped: its deaths log as "You have been
// slain", never in the third person.
func (r *GroupRoster) ObserveGroup(character string, names []string) {
	if character == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	s := r.stateLocked(character)
	s.pipeAt = now
	for _, n := range names {
		if n == "" || strings.EqualFold(n, character) {
			continue
		}
		s.members[strings.ToLower(n)] = now
	}
	for n, at := range s.members {
		if now.Sub(at) > rosterMemberTTL {
			delete(s.members, n)
		}
	}
}

// Check reports whether name is in character's group. known is false when
// there's no live Zeal pipe for character, so the caller can't tell and
// should fall back to its log-only behaviour.
func (r *GroupRoster) Check(character, name string) (member, known bool) {
	if character == "" {
		return false, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.chars[strings.ToLower(character)]
	if s == nil {
		return false, false
	}
	now := r.now()
	if now.Sub(s.pipeAt) > rosterPipeLive {
		return false, false
	}
	at, ok := s.members[strings.ToLower(name)]
	return ok && now.Sub(at) <= rosterMemberTTL, true
}
