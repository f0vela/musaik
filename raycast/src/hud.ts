import { showHUD } from "@raycast/api";
import { errorMessage, State } from "./api";

export async function runAction(action: () => Promise<State>, message: (st: State) => string) {
  try {
    await showHUD(message(await action()));
  } catch (err) {
    await showHUD(`⚠️ ${errorMessage(err)}`);
  }
}

export const trackLabel = (st: State) => (st.song ? `${st.song.title} — ${st.song.artist}` : "Nada en reproducción");
