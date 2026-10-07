import narration from "./narration.json";

// The narration drives the whole video: every scene lasts as long as its
// lines take to say, and the animations inside a scene fire on the words they
// illustrate. src/narration.json is written by scripts/narrate.py.

export const FPS = 30;
export const WIDTH = 1920;
export const HEIGHT = 1080;

export type Word = { text: string; startMs: number; endMs: number };
type Clip = { id: string; scene: string; text: string; file: string; durationMs: number; words: Word[] };
/** A clip placed on the video: `from` and `frames` are absolute. */
export type PlacedClip = Clip & { from: number; frames: number };

export const SCENES = ["ecosystem", "brand", "download", "qr", "choose", "features", "warning", "cta"] as const;
export type SceneId = (typeof SCENES)[number];
export type PlacedScene = { id: SceneId; from: number; frames: number; clips: PlacedClip[] };

const sec = (s: number) => Math.round(s * FPS);

// Seconds of picture before a scene's first line and after its last one.
const LEAD: Record<SceneId, number> = {
  ecosystem: 1.2, brand: 0.5, download: 0.5, qr: 0.6, choose: 0.5, features: 0.5, warning: 0.6, cta: 0.5,
};
const TAIL: Record<SceneId, number> = {
  ecosystem: 0.7, brand: 1.0, download: 1.5, qr: 0.8, choose: 1.2, features: 1.2, warning: 1.0, cta: 3.5,
};
// The pause before a line, where the picture needs time of its own.
const PAUSE: Record<string, number> = {
  "02": 0.3, "03": 0.6, "04": 1.4, "08": 1.8, "10": 0.6, "11": 0, "12": 0.5, "13": 0.5, "15": 0.5, "16": 0.6, "17": 0.6,
  "19": 0.4,
};
const DEFAULT_PAUSE = 0.4;

const clips = narration as Clip[];

export const scenes: PlacedScene[] = (() => {
  let cursor = 0;
  return SCENES.map((id) => {
    const from = cursor;
    let t = from + sec(LEAD[id]);
    const placed = clips
      .filter((c) => c.scene === id)
      .map((c, i) => {
        if (i > 0) t += sec(PAUSE[c.id] ?? DEFAULT_PAUSE);
        const frames = Math.ceil((c.durationMs / 1000) * FPS);
        const clip = { ...c, from: t, frames };
        t += frames;
        return clip;
      });
    t += sec(TAIL[id]);
    cursor = t;
    return { id, from, frames: t - from, clips: placed };
  });
})();

export const TOTAL_FRAMES = scenes[scenes.length - 1].from + scenes[scenes.length - 1].frames;
export const allClips = scenes.flatMap((s) => s.clips);

export const scene = (id: SceneId) => scenes.find((s) => s.id === id)!;

export const norm = (w: string) =>
  w.normalize("NFKD").replace(/[̀-ͯ]/g, "").toLowerCase().replace(/[^a-z0-9]/g, "");

const clipById = (id: string) => {
  const clip = allClips.find((c) => c.id === id);
  if (!clip) throw new Error(`no clip ${id}`);
  return clip;
};

/** The absolute frame at which a clip starts. */
export const clipStart = (id: string) => clipById(id).from;
/** The absolute frame at which a clip ends. */
export const clipEnd = (id: string) => clipById(id).from + clipById(id).frames;

/** The absolute frame at which the nth word starting with `match` is said. */
export function wordAt(id: string, match: string, nth = 0): number {
  const clip = clipById(id);
  const hits = clip.words.filter((w) => norm(w.text).startsWith(norm(match)));
  const word = hits[nth];
  if (!word) throw new Error(`no "${match}" in clip ${id}: ${clip.text}`);
  return clip.from + Math.round((word.startMs / 1000) * FPS);
}

/** The absolute frame at which that word ends. */
export function wordEnd(id: string, match: string, nth = 0): number {
  const clip = clipById(id);
  const word = clip.words.filter((w) => norm(w.text).startsWith(norm(match)))[nth];
  if (!word) throw new Error(`no "${match}" in clip ${id}`);
  return clip.from + Math.round((word.endMs / 1000) * FPS);
}
