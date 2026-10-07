import React from "react";
import { AbsoluteFill, interpolate, useCurrentFrame } from "remotion";
import { BrandMark, Rise, SiteIcon } from "../components";
import { C, ease, lerp, mono, pop } from "../theme";
import { scene, wordAt } from "../timeline";

// "O WhatsApp MCP é gratuito e tem código aberto no GitHub. Baixe agora mesmo,
// para Mac, Windows ou Linux, no link aqui na tela."

const clamp = { extrapolateLeft: "clamp", extrapolateRight: "clamp" } as const;

const SYSTEMS = [
  { icon: "APPLE", name: "macOS", word: "Mac" },
  { icon: "WINDOWS", name: "Windows", word: "Windows" },
  { icon: "LINUX", name: "Linux", word: "Linux" },
] as const;

export const Cta: React.FC = () => {
  const f = useCurrentFrame();
  const s = scene("cta");
  const at = (abs: number) => abs - s.from;

  const free = at(wordAt("18", "gratuito")) - 4;
  const open = at(wordAt("18", "codigo")) - 4;
  const github = at(wordAt("18", "GitHub")) - 4;
  const download = at(wordAt("19", "Baixe")) - 4;
  const link = at(wordAt("19", "link"));

  const url = pop(f, download, 13);
  const glow = 0.5 + 0.5 * Math.sin((f - link) / 9);
  const nudge = interpolate(f, [link, link + 6, link + 16], [0, 1, 0], clamp);

  return (
    <AbsoluteFill style={{ alignItems: "center" }}>
      <div style={{ position: "absolute", top: 70, display: "flex", alignItems: "center", gap: 28 }}>
        <BrandMark size={120} draw={ease(f, 0, 26)} />
        <Rise at={6} distance={30}>
          <div style={{ fontSize: 100, fontWeight: 800, letterSpacing: "-0.045em", color: "#f6fcf9" }}>WhatsApp MCP</div>
        </Rise>
      </div>

      <div style={{ position: "absolute", top: 260, display: "flex", gap: 20 }}>
        {[
          { at: free, content: <>Grátis</> },
          {
            at: open,
            content: (
              <>
                <SiteIcon icon="GITHUB" size={34} />
                Código aberto
              </>
            ),
          },
        ].map((chip, i) => {
          const t = pop(f, chip.at, 14);
          return (
            <div
              key={i}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 14,
                padding: "14px 30px",
                borderRadius: 999,
                border: `2px solid rgba(47,201,160,0.55)`,
                background: "rgba(47,201,160,0.1)",
                color: C.mintStrong,
                fontSize: 36,
                fontWeight: 700,
                opacity: interpolate(t, [0, 0.3], [0, 1], clamp),
                transform: `scale(${lerp(0.7, 1, t)})`,
              }}
            >
              {chip.content}
            </div>
          );
        })}
      </div>
      <Rise at={github} style={{ position: "absolute", top: 372, fontFamily: mono, fontSize: 30, color: C.onDeepSoft }}>
        github.com/BrOrlandi/whatsapp-mcp-local
      </Rise>

      {/* The link */}
      <div
        style={{
          position: "absolute",
          top: 480,
          display: "flex",
          alignItems: "center",
          gap: 22,
          padding: "30px 52px 30px 40px",
          borderRadius: 999,
          background: C.mint,
          color: C.mintInk,
          fontSize: 62,
          fontWeight: 800,
          letterSpacing: "-0.025em",
          boxShadow: `0 0 ${50 + glow * 40}px rgba(47,201,160,${0.35 + glow * 0.25}), 0 30px 60px -20px rgba(0,0,0,0.5)`,
          opacity: interpolate(url, [0, 0.3], [0, 1], clamp),
          transform: `scale(${lerp(0.6, 1, url) * (1 + nudge * 0.06)})`,
        }}
      >
        <SiteIcon icon="DOWNLOAD" size={60} />
        whatsapp-mcp.brorlandi.xyz
      </div>

      <div style={{ position: "absolute", top: 680, display: "flex", gap: 64 }}>
        {SYSTEMS.map((sys) => {
          const t = pop(f, at(wordAt("19", sys.word)) - 4, 14);
          return (
            <div
              key={sys.name}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 16,
                fontSize: 40,
                fontWeight: 700,
                color: "#f6fcf9",
                opacity: interpolate(t, [0, 0.3], [0, 1], clamp),
                transform: `translateY(${(1 - t) * 30}px)`,
              }}
            >
              <SiteIcon icon={sys.icon} size={46} color="#f6fcf9" />
              {sys.name}
            </div>
          );
        })}
      </div>
    </AbsoluteFill>
  );
};
