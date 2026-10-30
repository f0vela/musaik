import { readFile } from "fs/promises";
import { homedir } from "os";
import { join } from "path";

export type Song = {
  id: string;
  title: string;
  artist: string;
  album: string;
  duration: number;
  favorite: boolean;
};

export type State = {
  playing: boolean;
  paused: boolean;
  position: number;
  duration: number;
  volume: number;
  shuffle: boolean;
  repeat: string;
  queued: number;
  song?: Song;
};

const DISCOVERY = join(homedir(), "Library", "Application Support", "musaik", "remote.json");

export class MusaikError extends Error {}

async function connection(): Promise<{ base: string; token: string }> {
  let raw: string;
  try {
    raw = await readFile(DISCOVERY, "utf8");
  } catch {
    throw new MusaikError("Musaik no está listo: ábrelo y activa Ajustes → Integraciones → Control externo.");
  }
  try {
    const { port, token } = JSON.parse(raw) as { port: number; token: string };
    return { base: `http://127.0.0.1:${port}`, token };
  } catch {
    throw new MusaikError("remote.json está dañado: desactiva y vuelve a activar Control externo.");
  }
}

async function request<T>(method: "GET" | "POST", path: string): Promise<T> {
  const { base, token } = await connection();
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), 5000);
  try {
    const res = await fetch(base + path, {
      method,
      headers: { Authorization: `Bearer ${token}` },
      signal: ctrl.signal,
    });
    const body = (await res.json().catch(() => ({}))) as T & { error?: string };
    if (!res.ok) throw new MusaikError(body.error ?? `Musaik respondió ${res.status}`);
    return body;
  } catch (err) {
    if (err instanceof MusaikError) throw err;
    // remote.json outlives a crash, so a dead port means the app is not running.
    throw new MusaikError("No se pudo conectar con Musaik: ¿está abierto y con Control externo activo?");
  } finally {
    clearTimeout(timer);
  }
}

export const getState = () => request<State>("GET", "/state");
export const search = (q: string, limit = 30) =>
  request<Song[]>("GET", `/search?q=${encodeURIComponent(q)}&limit=${limit}`);
export const getQueue = () => request<Song[]>("GET", "/queue");
export const toggle = () => request<State>("POST", "/toggle");
export const next = () => request<State>("POST", "/next");
export const previous = () => request<State>("POST", "/prev");
export const favorite = () => request<State>("POST", "/favorite");
export const playSong = (id: string) => request<State>("POST", `/play/${encodeURIComponent(id)}`);
export const enqueue = (id: string) => request<State>("POST", `/queue/${encodeURIComponent(id)}`);
export const setVolume = (level: number) => request<State>("POST", `/volume?level=${level}`);

export function formatTime(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

export function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
