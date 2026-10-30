import { Action, ActionPanel, closeMainWindow, Icon, List, showHUD, showToast, Toast } from "@raycast/api";
import { useEffect, useState } from "react";
import { enqueue, errorMessage, formatTime, playSong, search, Song } from "./api";

export default function SearchSongs() {
  const [query, setQuery] = useState("");
  const [songs, setSongs] = useState<Song[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let stale = false;
    setLoading(true);
    search(query)
      .then((res) => {
        if (!stale) setSongs(res);
      })
      .catch((err) => {
        if (!stale) showToast({ style: Toast.Style.Failure, title: "Musaik", message: errorMessage(err) });
      })
      .finally(() => {
        if (!stale) setLoading(false);
      });
    return () => {
      stale = true;
    };
  }, [query]);

  return (
    <List isLoading={loading} throttle onSearchTextChange={setQuery} searchBarPlaceholder="Buscar canción, artista o álbum">
      {songs.map((song) => (
        <List.Item
          key={song.id}
          title={song.title}
          subtitle={song.artist}
          icon={song.favorite ? Icon.Heart : Icon.Music}
          accessories={[{ text: song.album }, { text: formatTime(song.duration) }]}
          actions={
            <ActionPanel>
              <Action
                title="Reproducir"
                icon={Icon.Play}
                onAction={async () => {
                  try {
                    await playSong(song.id);
                    await closeMainWindow({ clearRootSearch: true });
                    await showHUD(`▶️ ${song.title} — ${song.artist}`);
                  } catch (err) {
                    await showToast({ style: Toast.Style.Failure, title: "Musaik", message: errorMessage(err) });
                  }
                }}
              />
              <Action
                title="Añadir a la cola"
                icon={Icon.Plus}
                shortcut={{ modifiers: ["cmd"], key: "return" }}
                onAction={async () => {
                  try {
                    await enqueue(song.id);
                    await showToast({ style: Toast.Style.Success, title: "En cola", message: song.title });
                  } catch (err) {
                    await showToast({ style: Toast.Style.Failure, title: "Musaik", message: errorMessage(err) });
                  }
                }}
              />
            </ActionPanel>
          }
        />
      ))}
    </List>
  );
}
