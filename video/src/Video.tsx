import React, { useMemo } from "react";
import { Audio } from "@remotion/media";
import { AbsoluteFill, interpolate, Sequence, staticFile, useCurrentFrame } from "remotion";
import { Captions } from "./Captions";
import { Background } from "./components";
import { Brand } from "./scenes/Brand";
import { Choose } from "./scenes/Choose";
import { Cta } from "./scenes/Cta";
import { Download } from "./scenes/Download";
import { Ecosystem } from "./scenes/Ecosystem";
import { Features } from "./scenes/Features";
import { Qr } from "./scenes/Qr";
import { Warning } from "./scenes/Warning";
import { C, ease, font } from "./theme";
import { buildTimeline, SceneId, Timeline, TimelineContext, useTimeline } from "./timeline";
import { VERSIONS } from "./versions";

const SCENE: Record<SceneId, React.FC> = {
  ecosystem: Ecosystem,
  brand: Brand,
  download: Download,
  qr: Qr,
  choose: Choose,
  features: Features,
  warning: Warning,
  cta: Cta,
};

/** Fades a scene in from a slight blur and out into the next one. */
const Shell: React.FC<{ frames: number; last: boolean; children: React.ReactNode }> = ({ frames, last, children }) => {
  const f = useCurrentFrame();
  const { version } = useTimeline();
  const enter = ease(f, 0, version.enter);
  const leave = last ? 0 : interpolate(f, [frames - 2, frames + version.out], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <AbsoluteFill
      style={{
        opacity: enter * (1 - leave),
        transform: `translateY(${(1 - enter) * 18}px) scale(${1 - leave * 0.03})`,
        filter: enter < 1 || leave > 0 ? `blur(${(1 - enter) * 6 + leave * 8}px)` : undefined,
      }}
    >
      {children}
    </AbsoluteFill>
  );
};

// The music sits under the voice: lower while someone speaks, a little up in
// the pauses, and fading at both ends.
const musicVolume = (t: Timeline) => {
  const speaking = t.clips.map((c) => [c.from - 4, c.from + c.frames + 4] as const);
  return (f: number) => {
    const distance = Math.min(...speaking.map(([a, b]) => (f < a ? a - f : f > b ? f - b : 0)));
    const level = interpolate(distance, [0, 14], [0.06, 0.13], { extrapolateRight: "clamp" });
    const ends = Math.min(
      interpolate(f, [0, 30], [0, 1], { extrapolateRight: "clamp" }),
      interpolate(f, [t.total - 75, t.total - 4], [1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" }),
    );
    return level * ends;
  };
};

/** One version of the video, by its id. */
export const Explainer: React.FC<{ version: string }> = ({ version }) => {
  const timeline = useMemo(() => buildTimeline(VERSIONS.find((v) => v.id === version)!), [version]);
  return (
    <TimelineContext.Provider value={timeline}>
      <Cut />
    </TimelineContext.Provider>
  );
};

const Cut: React.FC = () => {
  const f = useCurrentFrame();
  const t = useTimeline();
  const w = t.scene("warning");
  const warm = interpolate(f, [w.from - 6, w.from + 20, w.from + w.frames - 6, w.from + w.frames + 14], [0, 1, 1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  return (
    <AbsoluteFill style={{ fontFamily: font, color: C.onDeep }}>
      <Background warm={warm} />
      {t.scenes.map((s, i) => {
        const Scene = SCENE[s.id];
        const last = i === t.scenes.length - 1;
        return (
          <Sequence key={s.id} name={s.id} from={s.from} durationInFrames={s.frames + (last ? 0 : t.version.out)}>
            <Shell frames={s.frames} last={last}>
              <Scene />
            </Shell>
          </Sequence>
        );
      })}
      <Captions />
      {t.clips.map((c) => (
        <Sequence key={c.id} name={`voz ${c.id}`} from={c.from} durationInFrames={c.frames + 2} layout="none">
          <Audio src={staticFile(c.file)} />
        </Sequence>
      ))}
      <Audio src={staticFile("music/bed.mp3")} volume={musicVolume(t)} />
    </AbsoluteFill>
  );
};
