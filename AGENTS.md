# Musaik — project rules

- Bump `version` in `mygo.json` on every change, then rebuild with `go tool mygo build` (produces `dist/darwin-arm64/Musaik.app` and `Musaik <version>.dmg`).
- Verify with `gofmt -w`, `go build ./...`, `go test ./...` before building.
- macOS Now Playing / media keys: implemented in `nowplaying_darwin.go` via purego/objc (`MPNowPlayingInfoCenter` + `MPRemoteCommandCenter`). Global F-key shortcuts are only a fallback when that init fails.
