import { loadFont } from "@remotion/google-fonts/SchibstedGrotesk";
import { loadFont as loadMono } from "@remotion/google-fonts/JetBrainsMono";
import { Easing, interpolate, spring } from "remotion";
import { FPS } from "./timeline";

// The landing page's palette (site/assets/site.css), so the video sits on the
// page as part of it.
export const C = {
  paper: "#eef3f1",
  ink: "#10241c",
  inkSoft: "#2c443d",
  brand: "#0b6b5d",
  brandSoft: "#dcefe9",
  deep: "#073d35",
  deepSunken: "#052e28",
  deepLine: "rgba(228, 241, 236, 0.16)",
  onDeep: "#eef8f4",
  onDeepSoft: "#b5d6cc",
  mint: "#2fc9a0",
  mintStrong: "#4adcb4",
  mintInk: "#04211b",
  aiBg: "#faf9f5",
  aiInk: "#1f1e1d",
  aiInkSoft: "#3d3c38",
  aiMuted: "#75736c",
  aiLine: "#e6e3da",
  aiBubble: "#f0eee6",
  amber: "#f6b94a",
  coral: "#ff8f7d",
  claude: "#d97757",
};

export const font = loadFont("normal", { weights: ["400", "500", "600", "700", "800"], subsets: ["latin", "latin-ext"] }).fontFamily;
export const mono = loadMono("normal", { weights: ["500"], subsets: ["latin"] }).fontFamily;

/** 0 → 1 with a soft overshoot, starting at `at`. */
export const pop = (frame: number, at: number, damping = 14) =>
  spring({ frame: frame - at, fps: FPS, config: { damping, stiffness: 170, mass: 0.8 } });

/** 0 → 1 without overshoot, starting at `at`. */
export const ease = (frame: number, at: number, frames = 18) =>
  interpolate(frame, [at, at + frames], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: Easing.bezier(0.2, 0.8, 0.2, 1),
  });

export const lerp = (a: number, b: number, t: number) => a + (b - a) * t;
