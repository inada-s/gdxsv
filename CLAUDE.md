# CLAUDE.md - gdxsv Development Guide

## Project Overview

gdxsv is a private game server for "Mobile Suit Gundam: Federation vs. Zeon & DX" (PS2/Dreamcast).
It replaces the original online service (ended 2004) to enable online multiplayer via emulators (Flycast, PCSX2) and real hardware.

## Architecture

### Components
- **LBS (Lobby Server)**: Authentication, lobby/room management, matchmaking (TCP:3333, HTTP:3380)
- **MCS (Match Server)**: Battle relay during gameplay (TCP:3334 + UDP)
- **Website**: React+TypeScript frontend deployed to GitHub Pages (gdxsv.net)
- **Cloud Functions**: On-demand MCS spawning, API, replay upload (GCP)
- **ChatOps Bot**: Discord/Slack integration (Python)

### Key Directories
```
gdxsv/           # Main Go server (LBS + MCS)
gdxsv/proto/     # Protocol Buffer definitions and generated code
website/          # React frontend (TypeScript)
infra/            # Deployment scripts, Cloud Functions, systemd services
chatops/          # Python chat ops bot
```

### Key Source Files
- `gdxsv/main.go` - Entry point, config, CLI commands (lbs, mcs, initdb, migratedb)
- `gdxsv/lbs.go` - Lobby Server core
- `gdxsv/lbs_handler.go` - Protocol message handlers (largest file, ~50KB)
- `gdxsv/lbs_lobby.go` - Lobby management and team shuffle
- `gdxsv/lbs_battle.go` - Battle matching logic
- `gdxsv/lbs_message.go` - Custom binary protocol (12-byte header, Shift-JIS)
- `gdxsv/mcs.go` - Match Server core
- `gdxsv/mcs_tcp.go` / `mcs_udp.go` - TCP/UDP battle communication
- `gdxsv/db.go` - Database interface
- `gdxsv/db_sqlite.go` - SQLite implementation (~27KB)
- `gdxsv/shareddata.go` - Shared state between LBS and MCS
- `gdxsv/rule.go` - Game rule serialization
- `gdxsv/patch.go` - Runtime game patches

## Build & Test Commands

```bash
make build          # Build binary to bin/gdxsv (requires CGO_ENABLED=1)
make test           # Run all tests with race detector: go test -race -v ./...
make lint           # Run golangci-lint
make fmt            # Format code: go fmt ./...
make install-tools  # Install protoc-gen-go and stringer
make protoc         # Regenerate protobuf code
make ci             # Full CI: build + test with coverage
```

### Running locally
```bash
./bin/gdxsv initdb          # Initialize database
./bin/gdxsv lbs -v=3        # Run LBS+MCS with debug logging
```

### Website
```bash
cd website && npm start     # Dev server
cd website && npm run build # Production build
```

## Code Conventions

### Go Style
- **Module**: `gdxsv` (see go.mod)
- **Go version**: 1.24+
- **Naming prefixes**:
  - `DB*` for database model structs (DBAccount, DBUser)
  - `M*` for master/config table structs (MLobbySetting, MRule, MPatch)
  - `Lbs*` for lobby server types (LbsPeer, LbsLobby, LbsBattle, LbsMessage)
- **Error handling**: Standard `if err != nil { return err }` pattern, wrapped with `github.com/pkg/errors`
- **Logging**: `go.uber.org/zap` structured logger, global `logger` variable
  - `logger.Info("msg", zap.String("key", val))`
  - `logger.Error("msg", zap.Error(err))`
- **Config**: Environment variables with `GDXSV_` prefix, parsed via struct tags (`github.com/caarlos0/env`)
- **Code generation**: `//go:generate stringer` for enum String() methods; protobuf via `make protoc`
- **Comments**: Written in English

### Import Order
1. Standard library
2. Internal packages (`gdxsv/gdxsv/proto`)
3. External packages (grouped)
4. Aliased imports last

### Database
- **SQLite** with WAL mode (`github.com/mattn/go-sqlite3`)
- Access via `github.com/jmoiron/sqlx`
- Interface defined in `db.go`, implementation in `db_sqlite.go`
- Main tables: `account`, `user`, `battle_record`, `m_lobby_setting`, `m_rule`, `m_patch`, `m_ban`

### Testing Patterns
- Standard `testing.T`, no external test frameworks
- **Table-driven tests** with `t.Run()` subtests
- In-memory SQLite for DB tests (`prepareTestDB()` in `main_test.go`)
- Custom helpers: `must(t, err)`, `assertEq(t, expected, got)`
- Mock types: `MockAddr`, `PipeConn` for network testing
- Some test files use numbered functions (Test001, Test002) for ordered execution

### Protocol
- **Lobby protocol**: Custom binary, 12-byte header (direction, category, command ID, body size, sequence, status)
- **String encoding**: Shift-JIS for Japanese text (`golang.org/x/text/encoding/japanese`)
- **Battle data**: Protocol Buffers (`gdxsv/proto/gdxsv.proto`)

## Linter Configuration

Uses golangci-lint v2. Only custom rule: errcheck excluded for `.Close()` calls.
See `.golangci.yml`.

## CI/CD

- GitHub Actions on push/PR to master
- Go tests with race detector on ubuntu-22.04
- golangci-lint check
- CodeQL analysis (Go, JavaScript, Python)
- Coverage uploaded to Codecov

## Workflow Rules

- After implementation, always run `make test` and `make lint` to verify.
- New features must include tests.
- For large or design-sensitive changes, use plan mode to propose an approach before implementing.
- Do not commit unless explicitly asked.
- For parallel tasks, use separate Claude Code sessions with worktrees (`/worktree`) to avoid conflicts.

### Commit & Pull Request
- **Commit messages**: Written in English, concise, focusing on "why" not "what".
- **Branch naming**: Use prefixed format: `feature/xxx`, `fix/xxx`, `refactor/xxx`.
- **PR target**: Always `master`.
- **PR title**: English, under 70 characters.
- **PR description**: English, include summary and test plan. Do not include auto-generated credit lines (e.g. "Generated with Claude Code").
- When asked to commit or create a PR, create a branch, commit, push, and open the PR via `gh pr create`.

## Cross-Repository: flycast

flycast (Dreamcast emulator) is a separate repository, not part of this repo.
Client-side changes may be needed when:
- Protobuf definitions (`gdxsv/proto/gdxsv.proto`) are modified → flycast C++ code must be regenerated
- Lobby protocol or patch handling changes → client-side behavior may need updating

flycast has its own CLAUDE.md. Do not modify flycast files from this repository.

## Important Notes

- CGO is required (SQLite driver). Ensure a C compiler is available.
- The `lbs_message.string.go` file is auto-generated by `stringer`. Do not edit manually.
- `gdxsv/proto/gdxsv.pb.go` is auto-generated by protoc. Do not edit manually.
- Game supports multiple platforms: PS2, DC1 (Dreamcast disc 1), DC2 (Dreamcast disc 2) with separate lobbies.
- Two factions: Renpo (Federation) and Zeon, each tracked independently in battle stats.
