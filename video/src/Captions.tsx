import React, { useMemo } from "react";
import { interpolate, useCurrentFrame } from "remotion";
import { FPS, PlacedClip, useTimeline } from "./timeline";
import { C } from "./theme";

// Subtitles burned into the picture, so the video explains itself with the
// sound off. They show the script's own words, timed by whisper, a sentence
// (or half a long one) at a time, with the word being said in mint.

type TimedWord = { text: string; from: number; to: number };
type Chunk = { words: TimedWord[]; from: number; to: number };

const MAX = 64; // characters; past this a sentence is split in two

const length = (words: TimedWord[]) => words.map((w) => w.text).join(" ").length;

// Words a line should not end on: they belong with what follows them.
const LEADING = new Set(["a", "o", "as", "os", "e", "ou", "de", "da", "do", "das", "dos", "em", "no", "na", "um", "uma", "com", "para", "por", "que", "à"]);

/**
 * Splits a run of words in two near its middle, preferring a comma and never
 * leaving an article or a preposition at the end of the first line.
 */
function halve(words: TimedWord[]): TimedWord[][] {
  if (length(words) <= MAX) return [words];
  const middle = length(words) / 2;
  let best = 1;
  let bestScore = Infinity;
  let pos = 0;
  for (let i = 0; i < words.length - 1; i++) {
    pos += words[i].text.length + 1;
    const comma = /[,;]$/.test(words[i].text);
    const dangling = LEADING.has(words[i].text.toLowerCase());
    const score = Math.abs(pos - middle) - (comma ? 14 : 0) + (dangling ? 30 : 0);
    if (score < bestScore) {
      bestScore = score;
      best = i + 1;
    }
  }
  return [...halve(words.slice(0, best)), ...halve(words.slice(best))];
}

function chunksOf(clip: PlacedClip): TimedWord[][] {
  const words = clip.words.map((w) => ({
    text: w.text,
    from: clip.from + Math.round((w.startMs / 1000) * FPS),
    to: clip.from + Math.round((w.endMs / 1000) * FPS),
  }));
  const sentences: TimedWord[][] = [[]];
  for (const w of words) {
    sentences[sentences.length - 1].push(w);
    if (/[.!?:…]$/.test(w.text)) sentences.push([]);
  }
  // A very short sentence ("Mas atenção.") reads better joined to the next one
  // only when both fit on one line.
  const merged: TimedWord[][] = [];
  for (const s of sentences.filter((s) => s.length)) {
    const prev = merged[merged.length - 1];
    if (prev && length(prev) < 22 && length(prev) + length(s) < MAX) prev.push(...s);
    else merged.push(s);
  }
  return merged.flatMap(halve);
}

function chunksFor(clips: PlacedClip[]): Chunk[] {
  const all = clips.flatMap(chunksOf).map((words) => ({ words, from: words[0].from - 2, to: words[words.length - 1].to }));
  // Hold each line a little after its last word, but never over the next one.
  return all.map((c, i) => {
    const next = all[i + 1];
    const hold = c.to + 14;
    return { ...c, to: next ? Math.min(hold, next.from) : hold };
  });
}

export const Captions: React.FC = () => {
  const f = useCurrentFrame();
  const { clips } = useTimeline();
  const chunks = useMemo(() => chunksFor(clips), [clips]);
  const chunk = chunks.find((c) => f >= c.from && f < c.to);
  if (!chunk) return null;
  const enter = interpolate(f, [chunk.from, chunk.from + 5], [0, 1], { extrapolateRight: "clamp" });
  const leave = interpolate(f, [chunk.to - 4, chunk.to], [1, 0], { extrapolateLeft: "clamp" });
  return (
    <div
      style={{
        position: "absolute",
        left: 0,
        right: 0,
        bottom: 54,
        display: "flex",
        justifyContent: "center",
        opacity: Math.min(enter, leave),
        transform: `translateY(${(1 - enter) * 10}px)`,
      }}
    >
      <div
        style={{
          maxWidth: 1480,
          padding: "14px 30px 16px",
          borderRadius: 18,
          background: "rgba(2, 18, 15, 0.8)",
          boxShadow: "0 12px 40px -12px rgba(0,0,0,0.6)",
          fontSize: 46,
          fontWeight: 600,
          lineHeight: 1.28,
          letterSpacing: "-0.005em",
          textAlign: "center",
          color: "#fff",
        }}
      >
        {chunk.words.map((w, i) => {
          const now = f >= w.from && f < Math.max(w.to, w.from + 4);
          return (
            <span key={i} style={{ color: now ? C.mintStrong : "#fff" }}>
              {w.text}
              {i < chunk.words.length - 1 ? " " : ""}
            </span>
          );
        })}
      </div>
    </div>
  );
};
