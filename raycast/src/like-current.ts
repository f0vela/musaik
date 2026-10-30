import { favorite } from "./api";
import { runAction } from "./hud";

export default () =>
  runAction(favorite, (st) =>
    st.song ? `${st.song.favorite ? "❤️ Like" : "💔 Sin like"}: ${st.song.title}` : "Nada en reproducción",
  );
