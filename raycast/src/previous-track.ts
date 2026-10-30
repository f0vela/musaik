import { previous } from "./api";
import { runAction, trackLabel } from "./hud";

export default () => runAction(previous, (st) => `⏮ ${trackLabel(st)}`);
