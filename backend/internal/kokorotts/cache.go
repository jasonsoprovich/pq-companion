package kokorotts

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strconv"

	"github.com/jasonsoprovich/pq-companion/backend/internal/tts"
)

// cacheKey is the content-addressed hash for a (base URL, voice, speed,
// text) tuple. It is namespaced with a "kokoro" prefix so the shared
// tts-cache dir (internal/tts) can hold this provider's entries — and
// Piper's — without collision, and so pointing at a different service or
// switching voice/speed never replays a stale cached phrase.
func cacheKey(cfg Config, text string) string {
	sum := sha256.Sum256([]byte("kokoro\x00" + cfg.EffectiveBaseURL() + "\x00" + cfg.EffectiveVoice() +
		"\x00" + strconv.FormatFloat(cfg.EffectiveSpeed(), 'f', -1, 64) + "\x00" + tts.NormalizeText(text)))
	return hex.EncodeToString(sum[:])
}

// cachePath returns the WAV path for a (base URL, voice, speed, text)
// tuple. It does not create the file or check for its existence.
func cachePath(baseDir string, cfg Config, text string) string {
	return filepath.Join(tts.CacheDir(baseDir), cacheKey(cfg, text)+".wav")
}
