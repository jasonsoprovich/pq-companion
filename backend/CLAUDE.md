# Go Backend Conventions
- Use standard library where possible, minimize dependencies
- Database queries live under `internal/db/` (and feature-specific subpackages where appropriate)
- API handlers in `internal/api/` — one file per resource
- Use structured logging (slog)
- Error handling: wrap errors with context, never panic in library code
- Tests: table-driven tests, test against real SQLite DB with test fixtures from `testdata/`
