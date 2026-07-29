package trigger

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

func TestIsValidDiscordWebhookURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://discord.com/api/webhooks/123456789012345678/abcDEF-123_xyz", true},
		{"https://discordapp.com/api/webhooks/123456789012345678/abcDEF-123_xyz", true},
		{"  https://discord.com/api/webhooks/1/a  ", true}, // surrounding whitespace trimmed
		{"http://discord.com/api/webhooks/1/a", false},     // not https
		{"https://evil.example.com/api/webhooks/1/a", false},
		{"https://discord.com/api/webhooks/1/a?wait=true", false}, // no trailing query/path allowed
		{"https://discord.com/api/webhooks/abc/a", false},         // id must be numeric
		{"", false},
		{"not a url", false},
	}
	for _, c := range cases {
		if got := isValidDiscordWebhookURL(c.url); got != c.want {
			t.Errorf("isValidDiscordWebhookURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

// TestEngine_DiscordWebhookFiresOnMatch verifies a discord_webhook action
// resolves its WebhookID via the injected resolver, substitutes captures
// into the message the same way overlay/TTS text does, and posts through
// sendWebhook (stubbed here so no real network call happens).
func TestEngine_DiscordWebhookFiresOnMatch(t *testing.T) {
	s, e := openTestEngine(t)

	type call struct{ url, content string }
	calls := make(chan call, 4)
	e.sendWebhook = func(url, content string) { calls <- call{url, content} }
	e.SetWebhookResolver(func(id string) (string, bool) {
		if id == "wh-1" {
			return "https://discord.com/api/webhooks/1/token", true
		}
		return "", false
	})

	tr := &Trigger{
		ID:      "guild-death",
		Name:    "Guild Death Broadcast",
		Enabled: true,
		Pattern: `Druzzil Ro tells the guild, '(.+) has killed (.+) in (.+)!'`,
		Actions: []Action{
			{Type: ActionDiscordWebhook, Text: "{2} just died", WebhookID: "wh-1"},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "Druzzil Ro tells the guild, 'Grokenspiel of <Seekers of Souls> has killed Lodizal in Iceclad Ocean!'")

	select {
	case got := <-calls:
		if got.url != "https://discord.com/api/webhooks/1/token" {
			t.Errorf("url = %q, want the resolved webhook URL", got.url)
		}
		if got.content != "Lodizal just died" {
			t.Errorf("content = %q, want %q", got.content, "Lodizal just died")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sendWebhook was not called within timeout")
	}
}

// TestEngine_DiscordWebhookSkipsUnresolvedID verifies a trigger referencing a
// deleted/unknown webhook never attempts to send — dispatchWebhooks must
// treat "resolver says not found" as a no-op, not a fallback URL.
func TestEngine_DiscordWebhookSkipsUnresolvedID(t *testing.T) {
	s, e := openTestEngine(t)

	calls := make(chan struct{}, 1)
	e.sendWebhook = func(url, content string) { calls <- struct{}{} }
	e.SetWebhookResolver(func(id string) (string, bool) { return "", false })

	tr := &Trigger{
		ID:      "dangling-webhook",
		Name:    "Dangling Webhook",
		Enabled: true,
		Pattern: `hello world`,
		Actions: []Action{
			{Type: ActionDiscordWebhook, Text: "hi", WebhookID: "deleted-id"},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "hello world")

	select {
	case <-calls:
		t.Fatal("sendWebhook should not be called when the webhook id doesn't resolve")
	case <-time.After(200 * time.Millisecond):
		// expected: nothing sent
	}
}

// TestEngine_DiscordWebhookNoResolverIsNoop verifies an engine with no
// SetWebhookResolver call (the default in most tests, and any environment
// where the feature hasn't been wired up) never panics on a discord_webhook
// action — it's silently inert, matching how a nil sink/activeChar behaves
// elsewhere in this engine.
func TestEngine_DiscordWebhookNoResolverIsNoop(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	e := NewEngine(s, hub, nil, nil)

	tr := &Trigger{
		ID:      "no-resolver",
		Name:    "No Resolver",
		Enabled: true,
		Pattern: `hello world`,
		Actions: []Action{
			{Type: ActionDiscordWebhook, Text: "hi", WebhookID: "wh-1"},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	// Must not panic.
	e.Handle(time.Now(), "hello world")
}
