import React from "react";
import { AbsoluteFill, interpolate, useCurrentFrame } from "remotion";
import { along, Cursor, SiteIcon, StepTitle } from "../components";
import { C, ease, lerp, pop } from "../theme";
import { useScene } from "../timeline";

// "Para começar, baixe o app. Tem versão para Mac, Windows e Linux."

const SYSTEMS = [
  { icon: "APPLE", name: "macOS", cue: "download.mac" },
  { icon: "WINDOWS", name: "Windows", cue: "download.windows" },
  { icon: "LINUX", name: "Linux", cue: "download.linux" },
] as const;

const CARD_W = 440;
const CARD_H = 420;
const GAP = 48;
const TOP = 300;
const left = (i: number) => 960 - (CARD_W * 3 + GAP * 2) / 2 + i * (CARD_W + GAP);

export const Download: React.FC = () => {
  const f = useCurrentFrame();
  const s = useScene("download");

  const button = { x: left(0) + CARD_W / 2, y: TOP + CARD_H - 74 };
  const click = s.cue("download.click");
  const [cx, cy] = along(f, [
    [click - 22, 1500, 980],
    [click - 2, button.x + 40, button.y + 6],
  ]);
  const cursorIn = ease(f, click - 24, 8) * (1 - ease(f, Math.max(click + 40, s.cue("download.end")), 10));
  const progress = interpolate(f, [click + 4, click + 30], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const done = ease(f, click + 30, 8);

  return (
    <AbsoluteFill>
      <StepTitle step={1} title="Baixe o app" align="center" y={86} />
      {SYSTEMS.map((sys, i) => {
        const t = pop(f, s.cue(sys.cue), 13);
        const isMac = i === 0;
        return (
          <div
            key={sys.name}
            style={{
              position: "absolute",
              left: left(i),
              top: TOP,
              width: CARD_W,
              height: CARD_H,
              borderRadius: 30,
              background: "#fff",
              color: C.ink,
              boxShadow: "0 40px 90px -30px rgba(0,0,0,0.6)",
              border: isMac && progress > 0 ? `3px solid ${C.brand}` : "3px solid transparent",
              padding: "44px 40px",
              display: "flex",
              flexDirection: "column",
              opacity: interpolate(t, [0, 0.3], [0, 1], { extrapolateRight: "clamp" }),
              transform: `translateY(${(1 - t) * 60}px) scale(${lerp(0.85, 1, t)})`,
            }}
          >
            <SiteIcon icon={sys.icon} size={88} color={C.ink} />
            <div style={{ marginTop: 28, fontSize: 56, fontWeight: 800, letterSpacing: "-0.03em" }}>{sys.name}</div>
            <div
              style={{
                marginTop: "auto",
                height: 76,
                borderRadius: 999,
                background: isMac && done > 0 ? C.mint : C.brand,
                color: isMac && done > 0 ? C.mintInk : "#fff",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                gap: 12,
                fontSize: 28,
                fontWeight: 700,
                position: "relative",
                overflow: "hidden",
              }}
            >
              {isMac && progress > 0 && done === 0 ? (
                <div style={{ position: "absolute", left: 0, top: 0, bottom: 0, width: `${progress * 100}%`, background: "rgba(255,255,255,0.22)" }} />
              ) : null}
              {isMac && done > 0 ? (
                <>
                  <svg width={30} height={30} viewBox="0 0 24 24">
                    <path d="M5 12.5l4.5 4.5L19 7.5" fill="none" stroke="currentColor" strokeWidth={2.8} strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                  Baixado
                </>
              ) : (
                <>
                  <SiteIcon icon="DOWNLOAD" size={28} />
                  Baixar
                </>
              )}
            </div>
          </div>
        );
      })}
      <Cursor x={cx} y={cy} clickAt={[click]} opacity={cursorIn} />
    </AbsoluteFill>
  );
};
