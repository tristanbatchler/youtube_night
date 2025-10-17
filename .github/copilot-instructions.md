**Architecture Snapshot**
- Go monolith started from `srv/cmd/main.go`; loads env from `srv/.env`, hydrates PostgreSQL via `db.GenSchema`, and wires session, store, state, and websocket dependencies before `internal.NewWebServer`.
- HTTP stack lives in `srv/internal/webServer.go`; routes are registered manually on `http.ServeMux` and handlers render `templ` components with HTMX-aware partials via `renderTemplate`.
- UI is server-rendered Templ (`srv/internal/templates/*.templ`) with Tailwind 4 utilities compiled into `srv/static/css/style.css`; HTMX is used for progressive enhancement and partial swaps.
- Real-time features rely on `srv/internal/websocket` where `Hub` coordinates gang rooms and keeps playback state in memory per gang.

**Directory Pointers**
- `srv/internal/stores` wrap sqlc-generated queries with validation, context timeouts, and logging; always call these instead of touching `db.Queries` directly.
- `srv/internal/states/game.go` holds the in-memory `GameStateManager`, keyed by gang id, and mediates video lists and submitter lookups once a game starts.
- `srv/internal/middleware` centralizes logging, content-type injection, and cookie-backed auth; `Auth` places `stores.SessionData` into request context for downstream access.
- `srv/internal/util` supplies helpers used inside templates (e.g. `util.If`, avatar emoji translations) to keep templ files declarative.
- `srv/internal/db/config` is the single source of truth for schema and sqlc queries; generated Go lives in `srv/internal/db/*.go` and should not be hand-edited.

**Backend Flow**
- Session cookies are HMAC-signed in `stores.SessionStore`; `CreateSessionCookie` (middleware) issues them and `Auth` enforces rotation, so reuse that flow when introducing new auth points.
- Gang lifecycle starts in `hostActionHandler`: create user → hash password with bcrypt → `GangStore.CreateGang` wraps a transaction to both create the gang and associate the host.
- Join flow (`joinActionHandler`) reuses existing users when name/avatar match and updates avatar if it drifts; keep that behaviour when extending join logic.
- YouTube integration happens in `searchVideosHandler` using `youtube.Service.Search.List`; expect 5 results and render via `templates.VideoSearchResults`.
- Video submissions use `VideoSubmissionStore.SubmitVideo`, which creates the video if missing (`CreateVideoIfNotExists`) and returns submission count for UI counters.
- Game lifecycle uses `startGameHandler` to shuffle submitted videos, purge prior guesses, hydrate members/submitters, seed websocket current video, and broadcast `game_start`.

**State & Realtime**
- `GameStateManager` plus websocket `Hub` maintain transient game state only in memory; restarting the process wipes active games, so handlers defensively rehydrate where possible.
- Websocket clients register per gang and receive JSON messages (`video_change`, `current_video`, `playback_state`); hosts are identified by `SessionData.IsHost` cross-checked via `UserStore.IsUserHostOfGang`.
- Playback sync now tracks the host-reported timestamp plus the last update instant inside `websocket.CurrentVideo`; late joiners recompute offsets from that data, so update those fields whenever you introduce new playback actions.
- Guessing endpoints (`submitGuessHandler`, `getGuessesHandler`) persist via `GuessStore`; `CreateVideoGuess` upserts so UI can allow revisions without extra delete flows.
- Logout clears the session cookie and, if the user is a host, calls `shutdownGame` to broadcast a stop and clear guesses.

**Developer Workflow**
- Before writing any HTMX code, read the [HTMX docs](../docs/htmx-docs.md) and the [HTMX refernece](../docs/htmx-reference.md).
- Before writing any Templ code, read the [Templ core concepts](../docs/templ-core-concepts.md), the [Templ HTMX example](../docs/templ-htmx-example.md), and the [Templ syntax and usage](../docs/templ-syntax-and-usage.md).
- Before writing any Hyperscript code, read the [Hyperscript reference](../docs/hyperscript-reference.md) and [Hyperscript docs](../docs/hyperscript-docs.md).
- Preferred dev loop: run VS Code task `Watch all` to spawn sqlc, Air (Go hot reload under Delve), Tailwind, and Templ watchers; individual tasks exist when you need only one tool.
- Manual commands mirror tasks: `$GOPATH/bin/sqlc generate -f srv/internal/db/config/sqlc.yml`, `$GOPATH/bin/templ generate`, and `npx @tailwindcss/cli -i srv/static/css/custom.css -o srv/static/css/style.css`.
- Air builds into `srv/build/main` with Delve headless debugging on `127.0.0.1:2345`; attach your debugger there rather than starting duplicate servers.
- Environment variables: README lists `SESSION_KEY`, but the Go code expects `SESSION_TOKEN`—set both or just `SESSION_TOKEN` in `srv/.env` to avoid startup failures.
- Database schema auto-applies on boot; when changing `schema.sql`/`queries.sql`, regenerate with sqlc and restart the server to pick up embedded SQL.

**Conventions & Gotchas**
- Only edit `.templ` sources; regenerate `_templ.go` files via the Templ CLI instead of hand-tweaking generated output.
- All store methods enforce short `context.WithTimeout` windows—propagate `r.Context()` and avoid long blocking calls in handlers to keep requests responsive.
- HTMX partials: handlers using `renderTemplate` must return appropriate status codes (422 for validation, etc.) because the layout is skipped on non-GET or HTMX requests.
- Avatar selections map emoji ↔ text through `util.AvatarEmojis`; keep that map in sync with any UI changes to prevent mismatched stored values.
- Websocket broadcasts rely on `Hub.BroadcastToGang`; when adding new message types, centralize JSON formatting alongside existing helpers to keep clients consistent.
- No automated tests exist; manual verification typically involves running a gang through join → submit → start game → guess, so script fixtures if you introduce breaking changes.
