import React, { CSSProperties } from "react";
import { AbsoluteFill, Easing, Img, interpolate, staticFile, useCurrentFrame } from "remotion";
import { C, ease, font, lerp, pop } from "./theme";
import * as icons from "./siteIcons";

const clamp = { extrapolateLeft: "clamp", extrapolateRight: "clamp" } as const;

/** The deep green of the landing page's header, with a slow drift of light. */
export const Background: React.FC<{ warm: number }> = ({ warm }) => {
  const f = useCurrentFrame();
  const x1 = 28 + Math.sin(f / 150) * 8;
  const y1 = 26 + Math.cos(f / 180) * 6;
  const x2 = 74 + Math.cos(f / 170) * 8;
  const y2 = 72 + Math.sin(f / 130) * 6;
  return (
    <AbsoluteFill style={{ background: C.deepSunken }}>
      <AbsoluteFill
        style={{
          background: `radial-gradient(1100px 760px at ${x1}% ${y1}%, rgba(47,201,160,0.15), transparent 62%),
            radial-gradient(1000px 700px at ${x2}% ${y2}%, rgba(11,107,93,0.5), transparent 66%),
            linear-gradient(160deg, #08463d 0%, #052e28 58%, #03221d 100%)`,
        }}
      />
      <AbsoluteFill
        style={{
          opacity: warm,
          background: `radial-gradient(1000px 700px at 50% 22%, rgba(246,185,74,0.2), transparent 64%),
            linear-gradient(160deg, #1f2a1d 0%, #16211a 60%, #101a15 100%)`,
        }}
      />
      <AbsoluteFill
        style={{
          backgroundImage: "radial-gradient(rgba(238,248,244,0.085) 1.5px, transparent 1.7px)",
          backgroundSize: "38px 38px",
          backgroundPosition: `${f * 0.12}px ${f * 0.08}px`,
          maskImage: "radial-gradient(ellipse at center, black 25%, transparent 80%)",
          WebkitMaskImage: "radial-gradient(ellipse at center, black 25%, transparent 80%)",
        }}
      />
    </AbsoluteFill>
  );
};

/** An icon from the landing page's sprite, painted with the current color. */
export const SiteIcon: React.FC<{ icon: keyof typeof icons; size: number; color?: string; style?: CSSProperties }> = ({
  icon,
  size,
  color,
  style,
}) => (
  <svg
    viewBox={icons[icon].viewBox}
    width={size}
    height={size}
    style={{ color, flex: "none", display: "block", ...style }}
    dangerouslySetInnerHTML={{ __html: icons[icon].body }}
  />
);

/** A brand logo on a white rounded tile, like an app icon. */
export const LogoTile: React.FC<{
  logo: string;
  size: number;
  pad?: number;
  gray?: number;
  glow?: string;
  style?: CSSProperties;
}> = ({ logo, size, pad = 0.22, gray = 0, glow, style }) => (
  <div
    style={{
      width: size,
      height: size,
      borderRadius: size * 0.27,
      background: "#fff",
      display: "grid",
      placeItems: "center",
      boxShadow: `0 ${size * 0.12}px ${size * 0.35}px -${size * 0.08}px rgba(0,0,0,0.45)${glow ? `, 0 0 ${size * 0.45}px ${glow}` : ""}`,
      filter: gray ? `grayscale(${gray})` : undefined,
      ...style,
    }}
  >
    <Img
      src={staticFile(`logos/${logo}.svg`)}
      style={{ width: size * (1 - pad * 2), height: size * (1 - pad * 2), objectFit: "contain" }}
    />
  </div>
);

/** The WhatsApp MCP mark, drawn stroke by stroke as `draw` goes 0 → 1. */
export const BrandMark: React.FC<{ size: number; draw?: number; color?: string; style?: CSSProperties }> = ({
  size,
  draw = 1,
  color = C.mint,
  style,
}) => {
  const outline = interpolate(draw, [0, 0.7], [0, 1], clamp);
  const inner = interpolate(draw, [0.45, 0.9], [0, 1], clamp);
  const dots = interpolate(draw, [0.75, 1], [0, 1], { ...clamp, easing: Easing.out(Easing.back(2)) });
  return (
    <svg viewBox="0 0 64 64" width={size} height={size} style={{ color, display: "block", overflow: "visible", ...style }}>
      <g fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round">
        <path
          d="M20 9h24a12 12 0 0 1 12 12v11a12 12 0 0 1-12 12H26L15 54.5V43a12 12 0 0 1-7-11V21A12 12 0 0 1 20 9Z"
          strokeWidth={5}
          pathLength={1}
          strokeDasharray={1}
          strokeDashoffset={1 - outline}
        />
        <path d="M19 21l13 6 13-6M32 27v8.5" strokeWidth={3.6} pathLength={1} strokeDasharray={1} strokeDashoffset={1 - inner} />
      </g>
      <g fill="currentColor">
        {[
          [19, 21, 3.6],
          [45, 21, 3.6],
          [32, 35.5, 3.6],
          [32, 27, 4.4],
        ].map(([cx, cy, r]) => (
          <circle key={`${cx}-${cy}`} cx={cx} cy={cy} r={r * dots} />
        ))}
      </g>
    </svg>
  );
};

export const APP_BG = "#0c1513";
export const TITLE_BAR = 42;

/** A macOS window around a screen of the app. */
export const AppWindow: React.FC<{
  x: number;
  y: number;
  width: number;
  height: number;
  title?: string;
  children: React.ReactNode;
  style?: CSSProperties;
}> = ({ x, y, width, height, title = "WhatsApp MCP", children, style }) => (
  <div
    style={{
      position: "absolute",
      left: x,
      top: y,
      width,
      height,
      borderRadius: 16,
      overflow: "hidden",
      background: APP_BG,
      boxShadow: "0 50px 120px -30px rgba(0,0,0,0.7), 0 0 0 1px rgba(255,255,255,0.09)",
      ...style,
    }}
  >
    <div
      style={{
        height: TITLE_BAR,
        background: "#1c2120",
        borderBottom: "1px solid rgba(255,255,255,0.06)",
        display: "flex",
        alignItems: "center",
        padding: "0 16px",
        position: "relative",
      }}
    >
      {["#ff5f57", "#febc2e", "#28c840"].map((c) => (
        <div key={c} style={{ width: 13, height: 13, borderRadius: 7, background: c, marginRight: 8 }} />
      ))}
      <div
        style={{
          position: "absolute",
          inset: 0,
          display: "grid",
          placeItems: "center",
          fontSize: 16,
          fontWeight: 600,
          color: "#a6b0ad",
          fontFamily: "-apple-system, BlinkMacSystemFont, 'Helvetica Neue', sans-serif",
        }}
      >
        {title}
      </div>
    </div>
    <div style={{ position: "relative", width, height: height - TITLE_BAR, overflow: "hidden" }}>{children}</div>
  </div>
);

/**
 * A screenshot of the panel, captured at twice its CSS size, filling the
 * window's width. `scroll` is in the page's CSS pixels.
 */
export const Shot: React.FC<{ src: string; width: number; scroll?: number; opacity?: number }> = ({
  src,
  width,
  scroll = 0,
  opacity = 1,
}) => (
  <Img
    src={staticFile(`shots/${src}`)}
    style={{
      position: "absolute",
      left: 0,
      top: 0,
      width,
      transform: `translateY(${-scroll * (width / 1000)}px)`,
      opacity,
    }}
  />
);

/** The mouse pointer, with a ripple when it clicks. */
export const Cursor: React.FC<{ x: number; y: number; clickAt?: number[]; opacity?: number }> = ({
  x,
  y,
  clickAt = [],
  opacity = 1,
}) => {
  const f = useCurrentFrame();
  const since = clickAt.map((c) => f - c).filter((d) => d >= 0 && d < 20);
  const press = since.length ? interpolate(since[0], [0, 3, 8], [1, 0.82, 1], clamp) : 1;
  return (
    <div style={{ position: "absolute", left: x, top: y, opacity, pointerEvents: "none", zIndex: 50 }}>
      {since.map((d) => (
        <div
          key={d}
          style={{
            position: "absolute",
            left: -32,
            top: -32,
            width: 64,
            height: 64,
            borderRadius: 32,
            border: `3px solid ${C.mint}`,
            transform: `scale(${interpolate(d, [0, 18], [0.3, 1.5])})`,
            opacity: interpolate(d, [0, 18], [0.9, 0]),
          }}
        />
      ))}
      <svg
        width={44}
        height={44}
        viewBox="0 0 28 28"
        style={{ position: "absolute", left: -6, top: -4, transform: `scale(${press})`, transformOrigin: "6px 4px", filter: "drop-shadow(0 4px 8px rgba(0,0,0,0.45))" }}
      >
        <path d="M6 4 L6 22 L10.4 17.9 L13.6 24.6 L16.6 23.3 L13.5 16.7 L19.4 16.7 Z" fill="#fff" stroke="#111" strokeWidth={1.4} strokeLinejoin="round" />
      </svg>
    </div>
  );
};

/** Moves along keyframes of [frame, x, y], easing between each pair. */
export const along = (f: number, path: [number, number, number][]): [number, number] => {
  // Cues close together could put a keyframe before the one it follows: each
  // keyframe comes at least a frame after the previous one.
  const keys = path.map(([k, x, y]) => [k, x, y] as [number, number, number]);
  for (let i = 1; i < keys.length; i++) keys[i][0] = Math.max(keys[i][0], keys[i - 1][0] + 1);
  if (f <= keys[0][0]) return [keys[0][1], keys[0][2]];
  for (let i = 1; i < keys.length; i++) {
    const [f1, x1, y1] = keys[i];
    const [f0, x0, y0] = keys[i - 1];
    if (f <= f1) {
      const t = interpolate(f, [f0, f1], [0, 1], { ...clamp, easing: Easing.bezier(0.45, 0, 0.2, 1) });
      return [lerp(x0, x1, t), lerp(y0, y1, t)];
    }
  }
  const last = keys[keys.length - 1];
  return [last[1], last[2]];
};

/** The "Passo N" label and title at the top of a step. */
export const StepTitle: React.FC<{ step: number; title: string; at?: number; x?: number; y?: number; align?: "left" | "center" }> = ({
  step,
  title,
  at = 4,
  x = 140,
  y = 70,
  align = "left",
}) => {
  const f = useCurrentFrame();
  const t = ease(f, at, 20);
  const t2 = ease(f, at + 5, 20);
  return (
    <div
      style={{
        position: "absolute",
        left: align === "center" ? 0 : x,
        right: align === "center" ? 0 : undefined,
        top: y,
        display: "flex",
        flexDirection: "column",
        alignItems: align === "center" ? "center" : "flex-start",
        gap: 14,
      }}
    >
      <div
        style={{
          opacity: t,
          transform: `translateY(${(1 - t) * 16}px)`,
          display: "flex",
          alignItems: "center",
          gap: 12,
          fontSize: 24,
          fontWeight: 700,
          letterSpacing: "0.02em",
          color: C.mint,
          textTransform: "uppercase",
        }}
      >
        <div
          style={{
            width: 40,
            height: 40,
            borderRadius: 20,
            background: C.mint,
            color: C.mintInk,
            display: "grid",
            placeItems: "center",
            fontSize: 22,
            fontWeight: 800,
          }}
        >
          {step}
        </div>
        Passo {step} de 3
      </div>
      <div
        style={{
          opacity: t2,
          transform: `translateY(${(1 - t2) * 20}px)`,
          fontSize: 58,
          fontWeight: 800,
          letterSpacing: "-0.03em",
          lineHeight: 1.02,
          color: "#f6fcf9",
        }}
      >
        {title}
      </div>
    </div>
  );
};

/** Text that rises into place at `at`. */
export const Rise: React.FC<{ at: number; children: React.ReactNode; style?: CSSProperties; distance?: number; frames?: number }> = ({
  at,
  children,
  style,
  distance = 24,
  frames = 18,
}) => {
  const f = useCurrentFrame();
  const t = ease(f, at, frames);
  return <div style={{ opacity: t, transform: `translateY(${(1 - t) * distance}px)`, ...style }}>{children}</div>;
};

/** Scales in with a spring at `at`. */
export const Pop: React.FC<{ at: number; children: React.ReactNode; style?: CSSProperties; from?: number }> = ({
  at,
  children,
  style,
  from = 0.6,
}) => {
  const f = useCurrentFrame();
  const t = pop(f, at);
  return (
    <div style={{ opacity: interpolate(t, [0, 0.4], [0, 1], clamp), transform: `scale(${lerp(from, 1, t)})`, ...style }}>{children}</div>
  );
};

export const fontStack = font;
