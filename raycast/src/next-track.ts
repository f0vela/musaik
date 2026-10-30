import { next } from "./api";
import { runAction, trackLabel } from "./hud";

export default () => runAction(next, (st) => `⏭ ${trackLabel(st)}`);
