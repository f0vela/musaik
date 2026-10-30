import { Icon, List, showToast, Toast } from "@raycast/api";
import { useEffect, useState } from "react";
import { errorMessage, formatTime, getQueue, Song } from "./api";

export default function Queue() {
  const [songs, setSongs] = useState<Song[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    getQueue()
      .then(setSongs)
      .catch((err) => showToast({ style: Toast.Style.Failure, title: "Musaik", message: errorMessage(err) }))
      .finally(() => setLoading(false));
  }, []);

  return (
    <List isLoading={loading}>
      <List.EmptyView title="La cola está vacía" icon={Icon.List} />
      {songs.map((song, i) => (
        <List.Item
          key={`${song.id}-${i}`}
          title={song.title}
          subtitle={song.artist}
          accessories={[{ text: song.album }, { text: formatTime(song.duration) }]}
        />
      ))}
    </List>
  );
}
