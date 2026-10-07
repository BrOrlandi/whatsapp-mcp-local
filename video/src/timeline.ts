import { createContext, useContext } from "react";

// The narration drives the whole video: every scene lasts as long as its
// lines take to say, and the animations inside a scene fire on cues, the words
// they illustrate. Each version of the video (src/versions.ts) has its own
// narration, written by scripts/narrate.py, its own pacing and its own cues.

export const FPS = 30;
export const WIDTH = 1920;
export const HEIGHT = 1080;

export type Word = { text: string; startMs: number; endMs: number };
export type Clip = { id: string; scene: string; text: string; file: string; durationMs: number; words: Word[] };
/** A clip placed on the video: `from` and `frames` are absolute. */
export type PlacedClip = Clip & { from: number; frames: number };

export const SCENES = ["ecosystem", "brand", "download", "qr", "choose", "features", "warning", "cta"] as const;
export type SceneId = (typeof SCENES)[number];
export type PlacedScene = { id: SceneId; from: number; frames: number; clips: PlacedClip[] };

/**
 * Where a cue falls: on the nth word of a line starting with `word`, or at the
 * line's start or end; `offset` moves it by that many frames.
 */
export type Cue = { clip: string; word?: string; nth?: number; at?: "start" | "end"; offset?: number };

export type Version = {
  id: string;
  narration: Clip[];
  /** Seconds of picture before a scene's first line and after its last one. */
  lead: Record<SceneId, number>;
  tail: Record<SceneId, number>;
  /** Seconds of pause before a line, by clip; `pause` elsewhere. */
  pauses: Record<string, number>;
  pause: number;
  /** Frames a scene takes to appear, and lingers into the next one while it fades. */
  enter: number;
  out: number;
  cues: Record<string, Cue>;
};

export type Timeline = {
  version: Version;
  scenes: PlacedScene[];
  clips: PlacedClip[];
  total: number;
  scene: (id: SceneId) => PlacedScene;
  /** The absolute frame of a cue. A cue the version lacks falls back to `fallback`, or fails. */
  cue: (name: string, fallback?: number) => number;
  has: (name: string) => boolean;
};

const sec = (s: number) => Math.round(s * FPS);

export const norm = (w: string) =>
  w.normalize("NFKD").replace(/[̀-ͯ]/g, "").toLowerCase().replace(/[^a-z0-9]/g, "");

export function buildTimeline(version: Version): Timeline {
  let cursor = 0;
  const scenes: PlacedScene[] = SCENES.map((id) => {
    const from = cursor;
    let t = from + sec(version.lead[id]);
    const placed = version.narration
      .filter((c) => c.scene === id)
      .map((c, i) => {
        if (i > 0) t += sec(version.pauses[c.id] ?? version.pause);
        const frames = Math.ceil((c.durationMs / 1000) * FPS);
        const clip = { ...c, from: t, frames };
        t += frames;
        return clip;
      });
    t += sec(version.tail[id]);
    cursor = t;
    return { id, from, frames: t - from, clips: placed };
  });
  const clips = scenes.flatMap((s) => s.clips);

  const resolve = (name: string, c: Cue): number => {
    const clip = clips.find((x) => x.id === c.clip);
    if (!clip) throw new Error(`cue ${name}: no clip ${c.clip} in ${version.id}`);
    let frame: number;
    if (c.word) {
      const word = clip.words.filter((w) => norm(w.text).startsWith(norm(c.word!)))[c.nth ?? 0];
      if (!word) throw new Error(`cue ${name}: no "${c.word}" in ${version.id} clip ${c.clip}: ${clip.text}`);
      frame = clip.from + Math.round((word.startMs / 1000) * FPS);
    } else {
      frame = c.at === "end" ? clip.from + clip.frames : clip.from;
    }
    return frame + (c.offset ?? 0);
  };

  return {
    version,
    scenes,
    clips,
    total: cursor,
    scene: (id) => scenes.find((s) => s.id === id)!,
    cue: (name, fallback) => {
      const c = version.cues[name];
      if (c) return resolve(name, c);
      if (fallback !== undefined) return fallback;
      throw new Error(`no cue ${name} in ${version.id}`);
    },
    has: (name) => name in version.cues,
  };
}

export const TimelineContext = createContext<Timeline | null>(null);

export const useTimeline = () => {
  const timeline = useContext(TimelineContext);
  if (!timeline) throw new Error("useTimeline outside a version");
  return timeline;
};

/**
 * A scene's view of the timeline: its frames, and cues as frames local to it,
 * which is what useCurrentFrame() counts inside the scene.
 */
export const useScene = (id: SceneId) => {
  const t = useTimeline();
  const s = t.scene(id);
  return {
    ...s,
    cue: (name: string, fallback?: number) =>
      fallback === undefined ? t.cue(name) - s.from : t.cue(name, fallback + s.from) - s.from,
    has: t.has,
  };
};
