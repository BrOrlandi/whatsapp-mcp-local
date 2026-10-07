import React from "react";
import { AbsoluteFill, interpolate, useCurrentFrame } from "remotion";
import { LogoTile } from "../components";
import { C, ease, lerp, pop } from "../theme";
import { clipStart, scene, wordAt } from "../timeline";

// "Hoje, você conecta todas as suas ferramentas de trabalho à sua IA
// preferida" — the tools gather around Claude; "só falta o WhatsApp", alone in
// the corner; "resolvido" — it joins the ring.

const CX = 960;
const CY = 450;
const RX = 430;
const RY = 300;
const TILE = 104;

type Tool = { logo: string; label: string; word?: string };
const TOOLS: Tool[] = [
  { logo: "gmail", label: "Gmail", word: "Gmail" },
  { logo: "google-calendar", label: "Agenda", word: "agenda" },
  { logo: "google-drive", label: "Drive", word: "Drive" },
  { logo: "slack", label: "Slack", word: "Slack" },
  { logo: "notion", label: "Notion", word: "Notion" },
  { logo: "jira", label: "Jira", word: "Jira" },
  { logo: "github", label: "GitHub", word: "GitHub" },
  { logo: "figma", label: "Figma" },
  { logo: "linear", label: "Linear" },
  { logo: "asana", label: "Asana" },
];
// The WhatsApp takes this place in the ring when it joins.
const WA_SLOT = 3;
const WA_CORNER = { x: 1700, y: 770 };

const angle = (i: number, n: number) => ((-90 + (360 / n) * i) * Math.PI) / 180;
const onRing = (a: number) => ({ x: CX + RX * Math.cos(a), y: CY + RY * Math.sin(a) });

export const Ecosystem: React.FC = () => {
  const f = useCurrentFrame();
  const s = scene("ecosystem");
  const at = (abs: number) => abs - s.from;

  const gather = at(wordAt("01", "conecta")) - 6;
  const all = at(wordAt("02", "Tudo"));
  const alone = at(wordAt("03", "fora")) - 4;
  const named = at(wordAt("03", "WhatsApp"));
  const dim = ease(f, at(clipStart("03")), 20);
  const solved = at(wordAt("04", "resolvido")) - 2;
  const join = pop(f, solved, 18);

  const claudeIn = pop(f, 4);
  const claudeBump =
    interpolate(f, [all, all + 6, all + 22], [0, 1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" }) +
    interpolate(f, [solved + 12, solved + 18, solved + 36], [0, 1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });

  const tools = TOOLS.map((tool, i) => {
    const start = gather + i * 3;
    const t = pop(f, start, 15);
    const a = lerp(angle(i, TOOLS.length), angle(i < WA_SLOT ? i : i + 1, TOOLS.length + 1), join);
    const slot = onRing(a);
    const pos = { x: lerp(CX, slot.x, t), y: lerp(CY, slot.y, t) };
    const said = tool.word ? at(wordAt("02", tool.word)) : all;
    const bump = interpolate(f, [said, said + 5, said + 18], [0, 1, 0.25], {
      extrapolateLeft: "clamp",
      extrapolateRight: "clamp",
    });
    const lit = Math.max(ease(f, said, 10), ease(f, all, 10)) * (1 - dim * 0.55) + ease(f, solved + 14, 12) * dim * 0.55;
    const line = ease(f, start + 8, 16);
    return { tool, t, pos, bump, lit, line, start };
  });

  const waIn = pop(f, alone, 16);
  const waSlot = onRing(angle(WA_SLOT, TOOLS.length + 1));
  const wa = { x: lerp(WA_CORNER.x, waSlot.x, join), y: lerp(WA_CORNER.y, waSlot.y, join) };
  const waWiggle = Math.sin((f - named) / 2.2) * 7 * interpolate(f, [named, named + 22], [1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" }) * (f >= named ? 1 : 0);
  const waLine = ease(f, solved + 10, 14);
  const lonely = waIn * (1 - ease(f, solved, 8));

  // A wave of light out of Claude when everything is connected.
  const wave = (from: number) => {
    const p = interpolate(f, [from, from + 40], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
    return p > 0 && p < 1 ? (
      <div
        style={{
          position: "absolute",
          left: CX - 560 * p,
          top: CY - 400 * p,
          width: 1120 * p,
          height: 800 * p,
          borderRadius: "50%",
          border: `3px solid ${C.mint}`,
          opacity: (1 - p) * 0.6,
        }}
      />
    ) : null;
  };

  // Packets of light flowing from each connected tool into Claude.
  const packets = (from: { x: number; y: number }, start: number, seed: number, strength: number) => {
    if (f < start || strength <= 0.05) return null;
    return [0, 0.5].map((offset) => {
      const p = (((f - start) / 46 + offset + seed * 0.137) % 1 + 1) % 1;
      return (
        <circle
          key={offset}
          cx={lerp(from.x, CX, p)}
          cy={lerp(from.y, CY, p)}
          r={5}
          fill={C.mintStrong}
          opacity={Math.sin(Math.PI * p) * strength}
        />
      );
    });
  };

  return (
    <AbsoluteFill>
      {/* Claude's glow */}
      <div
        style={{
          position: "absolute",
          left: CX - 260,
          top: CY - 260,
          width: 520,
          height: 520,
          borderRadius: "50%",
          background: "radial-gradient(circle, rgba(217,119,87,0.32), rgba(217,119,87,0) 65%)",
          opacity: claudeIn * (0.8 + claudeBump * 0.6),
        }}
      />
      {wave(all)}
      {wave(solved + 14)}
      <svg width={1920} height={1080} style={{ position: "absolute", inset: 0 }}>
        {tools.map(({ pos, lit, line }, i) => (
          <line
            key={i}
            x1={pos.x}
            y1={pos.y}
            x2={lerp(pos.x, CX, line)}
            y2={lerp(pos.y, CY, line)}
            stroke={lit > 0.5 ? C.mint : "rgba(181,214,204,0.32)"}
            strokeOpacity={0.45 + lit * 0.5}
            strokeWidth={2 + lit * 1.5}
            strokeLinecap="round"
          />
        ))}
        {tools.map(({ pos, line, start, lit }, i) => (
          <g key={`p${i}`}>{packets(pos, start + 24, i, line * (0.35 + lit * 0.65))}</g>
        ))}
        {/* The WhatsApp: a line that doesn't reach while it's alone, a full one once it joins. */}
        <line
          x1={wa.x}
          y1={wa.y}
          x2={lerp(wa.x, CX, 0.3)}
          y2={lerp(wa.y, CY, 0.3)}
          stroke={C.coral}
          strokeWidth={3}
          strokeDasharray="4 12"
          strokeLinecap="round"
          opacity={lonely * 0.8}
        />
        <line
          x1={wa.x}
          y1={wa.y}
          x2={lerp(wa.x, CX, waLine)}
          y2={lerp(wa.y, CY, waLine)}
          stroke={C.mint}
          strokeWidth={4}
          strokeLinecap="round"
          opacity={waLine}
        />
        {packets(wa, solved + 22, 3, waLine)}
      </svg>

      {tools.map(({ tool, t, pos, bump }, i) => (
        <div
          key={tool.logo}
          style={{
            position: "absolute",
            left: pos.x - TILE / 2,
            top: pos.y - TILE / 2,
            width: TILE,
            opacity: interpolate(t, [0, 0.3], [0, 1], { extrapolateRight: "clamp" }),
            transform: `scale(${t * (1 + bump * 0.16)})`,
          }}
        >
          <LogoTile logo={tool.logo} size={TILE} glow={bump > 0.3 ? `rgba(47,201,160,${bump * 0.7})` : undefined} />
          <div
            style={{
              position: "absolute",
              top: TILE + 10,
              left: -40,
              right: -40,
              textAlign: "center",
              fontSize: 22,
              fontWeight: 600,
              color: bump > 0.2 ? "#fff" : C.onDeepSoft,
            }}
          >
            {tool.label}
          </div>
        </div>
      ))}

      {/* Claude, in the middle */}
      <div
        style={{
          position: "absolute",
          left: CX - 92,
          top: CY - 92,
          transform: `scale(${claudeIn * (1 + claudeBump * 0.1)})`,
        }}
      >
        <LogoTile logo="claude" size={184} pad={0.2} />
      </div>

      {/* The WhatsApp */}
      <div
        style={{
          position: "absolute",
          left: wa.x - TILE / 2,
          top: wa.y - TILE / 2,
          width: TILE,
          opacity: interpolate(waIn, [0, 0.3], [0, 1], { extrapolateRight: "clamp" }),
          transform: `scale(${waIn * (1 + interpolate(f, [solved, solved + 8, solved + 24], [0, 0.22, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" }))}) rotate(${waWiggle}deg)`,
        }}
      >
        <LogoTile
          logo="whatsapp"
          size={TILE}
          gray={1 - join}
          style={{ opacity: lerp(0.75, 1, join) }}
          glow={join > 0.2 ? `rgba(47,201,160,${join * 0.6})` : undefined}
        />
        <div
          style={{
            position: "absolute",
            top: TILE + 10,
            left: -60,
            right: -60,
            textAlign: "center",
            fontSize: 22,
            fontWeight: 700,
            color: "#fff",
          }}
        >
          WhatsApp
        </div>
        <div
          style={{
            position: "absolute",
            top: TILE + 46,
            left: -80,
            right: -80,
            display: "flex",
            justifyContent: "center",
            opacity: lonely,
          }}
        >
          <div
            style={{
              padding: "5px 14px",
              borderRadius: 999,
              border: `2px solid ${C.coral}`,
              color: C.coral,
              fontSize: 19,
              fontWeight: 700,
              whiteSpace: "nowrap",
            }}
          >
            Sem conexão
          </div>
        </div>
      </div>
    </AbsoluteFill>
  );
};
