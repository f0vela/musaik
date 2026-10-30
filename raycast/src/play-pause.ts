import { toggle } from "./api";
import { runAction, trackLabel } from "./hud";

export default () => runAction(toggle, (st) => `${st.playing ? "▶️" : "⏸"} ${trackLabel(st)}`);
