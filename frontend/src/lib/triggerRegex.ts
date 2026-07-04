// Trigger patterns are matched by the Go backend (RE2 via regexp.Compile), but
// the editor forms validate them in the browser with `new RegExp()` so the user
// gets immediate feedback. Go/RE2 accepts a few syntaxes JavaScript rejects, so
// translate the documented ones before validating — otherwise valid backend
// patterns (including several built-in pack patterns) get falsely flagged as
// "Invalid regular expression".
//
// Handled:
//   - Named groups:   (?P<name>…)  ->  (?<name>…)
//   - Leading inline flags: (?i) (?is) (?im) …  ->  RegExp flags argument
//     (JS has no inline-flag syntax; `i`, `m`, `s` map to flags. `U`/`s`-only-in-
//     Go quirks are best-effort — `U` has no JS equivalent so it's dropped.)
//
// Compiles the pattern the way JS can express it and returns the RegExp, or
// throws the same error `new RegExp` would — callers keep their existing
// try/catch and error-message handling.
export function compileTriggerRegex(pattern: string): RegExp {
  let src = pattern.replace(/\(\?P</g, '(?<')

  // Pull a leading global inline-flag group like (?i) or (?ims) off the front
  // and translate the JS-expressible flags. Anything else (mid-pattern inline
  // flags, Go-only `U`) is left for the backend, which is the real matcher.
  let flags = ''
  const m = src.match(/^\(\?([a-zA-Z]+)\)/)
  if (m) {
    for (const f of m[1]) {
      if ('ims'.includes(f) && !flags.includes(f)) flags += f
    }
    src = src.slice(m[0].length)
  }

  return new RegExp(src, flags)
}
