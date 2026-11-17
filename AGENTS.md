# Musaik — project rules

- Bump `version` in `mygo.json` on every change, then rebuild with `go tool mygo build` (produces `dist/darwin-arm64/Musaik.app` and `Musaik <version>.dmg`).
- Verify with `gofmt -w`, `go build ./...`, `go test ./...` before building.
- macOS Now Playing / media keys: implemented in `nowplaying_darwin.go` via purego/objc (`MPNowPlayingInfoCenter` + `MPRemoteCommandCenter`). Global F-key shortcuts are only a fallback when that init fails.
- Terminal app: `cmd/musaik-tui` (Bubble Tea v2) on top of `internal/core`, the headless engine. Keep its behavior in parity with the desktop app (see `docs/TUI.md`); build it with `go build -o dist/musaik-tui ./cmd/musaik-tui` and test it with `go test ./internal/core ./cmd/musaik-tui`. Its `version` follows `mygo.json`.
