import React from "react";
import { AbsoluteFill, interpolate, useCurrentFrame } from "remotion";
import { along, AppWindow, Cursor, LogoTile, Rise, Shot, StepTitle, TITLE_BAR } from "../components";
import { C, ease, lerp, mono, pop } from "../theme";
import { useScene } from "../timeline";

// "Agora, escolha a sua ferramenta de IA e siga o passo a passo." — "O Claude
// se conecta com um clique. Para o Codex, o Cursor e outras, o app dá um texto
// pronto para colar."

const WIN = { y: 200, w: 1080, h: 710 };
const K = WIN.w / 1000;
const SCROLL = 250; // CSS pixels: from the top of the page down to the list of tools

const OTHERS = [
  { logo: "openai", name: "Codex", cue: "choose.codex" },
  { logo: "cursor", name: "Cursor", cue: "choose.cursor" },
] as const;

export const Choose: React.FC = () => {
  const f = useCurrentFrame();
  const s = useScene("choose");

  const winIn = pop(f, 2, 16);
  const toSide = ease(f, s.cue("choose.side"), 22);
  const winX = lerp(420, 100, toSide);
  // Where a point of the page (in CSS pixels) is on the video, at a given scroll.
  const onScreen = (x: number, y: number, scroll = 0) => [winX + x * K, WIN.y + TITLE_BAR + (y - scroll) * K] as const;

  const scrollStart = s.cue("choose.scroll");
  const scroll = interpolate(f, [scrollStart, scrollStart + 26], [0, SCROLL], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: (t) => 1 - Math.pow(1 - t, 3),
  });
  const pick = s.cue("choose.pick");
  const add = s.cue("choose.add");
  const showSetup = ease(f, pick + 5, 8);
  const showDone = ease(f, add + 10, 8);

  const card = onScreen(500, 455, SCROLL);
  const button = onScreen(367, 253);
  const [cx, cy] = along(f, [
    [scrollStart + 20, 1500, 960],
    [pick - 3, card[0] + 60, card[1] + 6],
    [Math.max(pick + 12, add - 22), card[0] + 60, card[1] + 6],
    [add - 3, button[0] + 30, button[1] + 8],
  ]);
  const cursorOpacity = ease(f, scrollStart + 18, 8) * (1 - ease(f, add + 22, 10));

  const pasteCard = s.cue("choose.paste");
  const copied = s.cue("choose.copied");

  return (
    <AbsoluteFill>
      <StepTitle step={3} title="Conecte a sua IA" x={winX} y={44} />
      <AppWindow
        x={winX}
        y={WIN.y}
        width={WIN.w}
        height={WIN.h}
        style={{ opacity: winIn, transform: `translateY(${(1 - winIn) * 50}px)` }}
      >
        <Shot src="02-escolher-full.png" width={WIN.w} scroll={scroll} />
        <Shot src="03-claude-desktop.png" width={WIN.w} opacity={showSetup} />
        <Shot src="04-claude-connected.png" width={WIN.w} opacity={showDone} />
        {showDone > 0 ? (
          <div
            style={{
              position: "absolute",
              left: 180 * K,
              top: 134 * K,
              width: 640 * K,
              height: 64 * K,
              borderRadius: 14,
              boxShadow: `0 0 0 4px rgba(47,201,160,${0.9 * (1 - ease(f, add + 40, 20))}), 0 0 50px rgba(47,201,160,${0.5 * showDone})`,
            }}
          />
        ) : null}
      </AppWindow>
      <Cursor x={cx} y={cy} clickAt={[pick, add]} opacity={cursorOpacity} />

      {/* The other tools: a text to paste */}
      <div style={{ position: "absolute", left: 1250, top: WIN.y + 10, width: 600 }}>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 18 }}>
          {OTHERS.map((o) => {
            const t = pop(f, s.cue(o.cue), 13);
            return (
              <div
                key={o.name}
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 16,
                  padding: "14px 26px 14px 14px",
                  borderRadius: 24,
                  background: "rgba(238,248,244,0.07)",
                  border: `1px solid ${C.deepLine}`,
                  opacity: interpolate(t, [0, 0.3], [0, 1], { extrapolateRight: "clamp" }),
                  transform: `scale(${lerp(0.7, 1, t)})`,
                }}
              >
                <LogoTile logo={o.logo} size={64} pad={0.2} />
                <span style={{ fontSize: 34, fontWeight: 700 }}>{o.name}</span>
              </div>
            );
          })}
          {(() => {
            const t = pop(f, s.cue("choose.others"), 13);
            return (
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  padding: "0 26px",
                  borderRadius: 24,
                  border: `2px dashed ${C.deepLine}`,
                  fontSize: 30,
                  fontWeight: 600,
                  color: C.onDeepSoft,
                  whiteSpace: "nowrap",
                  opacity: interpolate(t, [0, 0.3], [0, 1], { extrapolateRight: "clamp" }),
                  transform: `scale(${lerp(0.7, 1, t)})`,
                }}
              >
                e outras
              </div>
            );
          })()}
        </div>
        <Rise at={pasteCard} distance={40} style={{ marginTop: 34 }}>
          <div
            style={{
              borderRadius: 24,
              background: "#0c1513",
              border: `1px solid rgba(255,255,255,0.1)`,
              boxShadow: "0 40px 90px -30px rgba(0,0,0,0.7)",
              padding: "26px 28px",
            }}
          >
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
              <span style={{ fontSize: 28, fontWeight: 700 }}>Texto pronto para colar</span>
              <span
                style={{
                  padding: "8px 20px",
                  borderRadius: 12,
                  fontSize: 24,
                  fontWeight: 700,
                  background: f >= copied ? C.mint : "transparent",
                  color: f >= copied ? C.mintInk : C.mint,
                  border: `2px solid ${C.mint}`,
                  transform: `scale(${interpolate(f, [copied, copied + 4, copied + 10], [1, 0.9, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" })})`,
                }}
              >
                {f >= copied ? "Copiado" : "Copiar"}
              </span>
            </div>
            {/* The opening of the text the app gives for "Outra ferramenta". */}
            <div
              style={{
                marginTop: 18,
                fontSize: 25,
                lineHeight: 1.5,
                color: "#cfe3dc",
                height: 150,
                overflow: "hidden",
                maskImage: "linear-gradient(black 55%, transparent)",
                WebkitMaskImage: "linear-gradient(black 55%, transparent)",
              }}
            >
              Quero conectar um servidor MCP (Model Context Protocol) em você, para que você possa ler e usar o meu
              WhatsApp. Configure isso para mim.{" "}
              <span style={{ fontFamily: mono, fontSize: 21, color: C.mint }}>URL: http://127.0.0.1:47821/mcp</span>
            </div>
          </div>
        </Rise>
      </div>
    </AbsoluteFill>
  );
};
