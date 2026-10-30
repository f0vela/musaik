# Musaik for Raycast / Tinycast

Controls Musaik through its local HTTP API (`remote.go`). Works in Raycast and in Tinycast (both run `@raycast/api` extensions).

## Setup

1. In Musaik (≥ 0.2.9): Settings → Integrations → enable **Permitir control externo**.
2. Build the extension:
   ```sh
   cd raycast
   npm install
   npm run build        # outputs to dist/
   ```
   `ray build` creates `~/.config/raycast`; if that folder doesn't exist (no Raycast installed), run it as `HOME=$(mktemp -d) npm run build`.
3. Import it:
   - **Tinycast**: import the `raycast/` folder (it must contain the built `dist/`).
   - **Raycast**: `npm run dev`, or Import Extension → this folder.

## Commands

Now Playing, Search Songs (Enter plays, ⌘↵ queues), Show Queue, Play / Pause, Next Track, Previous Track, Like Current Song.

## How it connects

Musaik writes `~/Library/Application Support/musaik/remote.json` (`{port, token}`, mode 0600) when the server starts, with a new token each time. The extension reads it on every call, so nothing needs configuring. Errors from a closed app or disabled setting are shown as a HUD/toast.
