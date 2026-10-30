import { Action, ActionPanel, Detail, Icon } from "@raycast/api";
import { useEffect, useState } from "react";
import { errorMessage, favorite, formatTime, getState, next, previous, State, toggle } from "./api";

export default function NowPlaying() {
  const [state, setState] = useState<State>();
  const [error, setError] = useState<string>();

  async function run(action: () => Promise<State>) {
    try {
      setState(await action());
      setError(undefined);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  // Poll only while the command is open; the interval dies with the view.
  useEffect(() => {
    run(getState);
    const timer = setInterval(() => run(getState), 1000);
    return () => clearInterval(timer);
  }, []);

  let markdown = "Cargando…";
  if (error) {
    markdown = `# Musaik no disponible\n\n${error}`;
  } else if (state) {
    const song = state.song;
    markdown = song
      ? `# ${song.title}\n\n**${song.artist}** · ${song.album}\n\n${state.paused ? "⏸" : "▶️"} ${formatTime(state.position)} / ${formatTime(state.duration)} · 🔊 ${state.volume}%${song.favorite ? " · ❤️" : ""}`
      : "# Nada en reproducción";
  }

  return (
    <Detail
      markdown={markdown}
      isLoading={!state && !error}
      actions={
        <ActionPanel>
          <Action title={state?.playing ? "Pausar" : "Reproducir"} icon={Icon.Play} onAction={() => run(toggle)} />
          <Action
            title="Siguiente"
            icon={Icon.Forward}
            shortcut={{ modifiers: ["cmd"], key: "arrowRight" }}
            onAction={() => run(next)}
          />
          <Action
            title="Anterior"
            icon={Icon.Rewind}
            shortcut={{ modifiers: ["cmd"], key: "arrowLeft" }}
            onAction={() => run(previous)}
          />
          <Action
            title="Like / Quitar like"
            icon={Icon.Heart}
            shortcut={{ modifiers: ["cmd"], key: "l" }}
            onAction={() => run(favorite)}
          />
        </ActionPanel>
      }
    />
  );
}
