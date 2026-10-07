import React from "react";
import { AbsoluteFill, interpolate, useCurrentFrame } from "remotion";
import { BrandMark, Rise } from "../components";
import { C, ease, lerp, pop } from "../theme";
import { clipStart, scene, wordAt } from "../timeline";

// "Feito isso, é só pedir. A sua IA pode ler as suas conversas e os seus
// grupos, achar aquela mensagem de meses atrás e resumir o que você perdeu.
// Ela também responde e envia mensagens, manda fotos e arquivos, transcreve
// áudios e até cria enquetes."

const ui = "-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif";
const CHAT = { x: 120, y: 150, w: 840, h: 720 };

const icon = (d: string) => (
  <svg width={34} height={34} viewBox="0 0 24 24">
    <path d={d} fill="none" stroke="currentColor" strokeWidth={1.9} strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

const FEATURES = [
  { label: "Ler conversas e grupos", clip: "12", word: "conversas", d: "M5 5h11a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2H9.5L6 19v-3H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2zM7 9.5h7M7 12.5h4.5" },
  { label: "Achar mensagens antigas", clip: "12", word: "achar", d: "M10.5 4a6.5 6.5 0 1 1 0 13 6.5 6.5 0 0 1 0-13zM15.4 15.4L20 20" },
  { label: "Resumir o que você perdeu", clip: "12", word: "resumir", d: "M5 6h14M5 10h14M5 14h9M5 18h6" },
  { label: "Responder e enviar mensagens", clip: "13", word: "responde", d: "M4 12L20 4l-6 16-2.6-6.4L4 12zM11.4 13.6L20 4" },
  { label: "Mandar fotos e arquivos", clip: "13", word: "fotos", d: "M5 4h14a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1zM4 16l4.5-4.5 4 4L15 13l5 5M15.5 8.5h.01" },
  { label: "Transcrever áudios", clip: "13", word: "transcreve", d: "M12 3a3 3 0 0 1 3 3v5a3 3 0 0 1-6 0V6a3 3 0 0 1 3-3zM5.5 11a6.5 6.5 0 0 0 13 0M12 17.5V21" },
  { label: "Criar enquetes", clip: "13", word: "enquetes", d: "M6 20V11M12 20V4M18 20v-6" },
];

/** What the composer shows while a request is being typed. */
const typed = (f: number, text: string, from: number, to: number) => {
  const n = Math.round(interpolate(f, [from, to], [0, text.length], { extrapolateLeft: "clamp", extrapolateRight: "clamp" }));
  return f >= from && f < to + 2 ? text.slice(0, n) : "";
};

const Ask: React.FC<{ at: number; children: React.ReactNode }> = ({ at, children }) => (
  <Rise at={at} distance={14} frames={10} style={{ alignSelf: "flex-end", maxWidth: "86%" }}>
    <div style={{ padding: "12px 20px", borderRadius: 22, background: C.aiBubble, fontSize: 25, lineHeight: 1.45 }}>{children}</div>
  </Rise>
);

const Tool: React.FC<{ at: number; done: number; busy: string; label: string }> = ({ at, done, busy, label }) => {
  const f = useCurrentFrame();
  return (
    <Rise at={at} distance={10} frames={10} style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 22, color: C.aiMuted, height: 32 }}>
      {f < done ? (
        <div
          style={{
            width: 20,
            height: 20,
            borderRadius: 10,
            border: "3px solid #d3cfc3",
            borderTopColor: C.aiInkSoft,
            transform: `rotate(${f * 14}deg)`,
          }}
        />
      ) : (
        <BrandMark size={24} color={C.brand} />
      )}
      <span>{f < done ? busy : label}</span>
      <svg width={18} height={18} viewBox="0 0 24 24">
        <path d="M9 6l6 6-6 6" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    </Rise>
  );
};

export const Features: React.FC = () => {
  const f = useCurrentFrame();
  const s = scene("features");
  const at = (abs: number) => abs - s.from;

  const ask1 = at(clipStart("11")) + 20;
  const read = at(wordAt("12", "ler"));
  const sum = at(wordAt("12", "resumir")) - 14;
  const ask2 = at(clipStart("13")) - 2;
  const sent = at(wordAt("13", "envia"));

  const composer =
    typed(f, "Resume o que rolou hoje no grupo da família", ask1 - 24, ask1 - 4) ||
    typed(f, "Avisa no grupo que eu levo as bebidas", ask2 - 22, ask2 - 4);
  const chatIn = pop(f, 0, 16);

  return (
    <AbsoluteFill>
      {/* An AI assistant's chat, like the one on the landing page */}
      <div
        style={{
          position: "absolute",
          left: CHAT.x,
          top: CHAT.y,
          width: CHAT.w,
          height: CHAT.h,
          borderRadius: 26,
          overflow: "hidden",
          background: C.aiBg,
          color: C.aiInk,
          fontFamily: ui,
          boxShadow: "0 50px 120px -30px rgba(0,0,0,0.7)",
          display: "flex",
          flexDirection: "column",
          opacity: chatIn,
          transform: `translateY(${(1 - chatIn) * 50}px)`,
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 14, padding: "16px 22px", borderBottom: `1px solid ${C.aiLine}` }}>
          <div style={{ display: "flex", gap: 8 }}>
            {[0, 1, 2].map((i) => (
              <div key={i} style={{ width: 13, height: 13, borderRadius: 7, background: "#d3cfc3" }} />
            ))}
          </div>
          <span style={{ fontSize: 20, fontWeight: 600, color: C.aiInkSoft }}>Resumo do grupo da família</span>
        </div>
        <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: 20, padding: "28px 30px 10px" }}>
          <Ask at={ask1}>Resume o que rolou hoje no grupo da família</Ask>
          <Tool at={read} done={read + 26} busy="Lendo as mensagens de Família…" label="Leu 52 mensagens de Família" />
          <div style={{ fontSize: 25, lineHeight: 1.45 }}>
            <Rise at={sum} distance={10} frames={10}>
              Hoje no grupo da família:
            </Rise>
            <ul style={{ margin: "8px 0 0", paddingLeft: 28 }}>
              {[
                ["Almoço de domingo:", "na casa da vó, às 13h."],
                ["Sobremesa:", "a Carla leva. Faltam as bebidas."],
                ["Fotos:", "o Pedro mandou 12 da formatura."],
              ].map(([b, rest], i) => (
                <Rise key={b} at={sum + 6 + i * 6} distance={10} frames={10}>
                  <li style={{ marginTop: 4 }}>
                    <strong style={{ fontWeight: 650 }}>{b}</strong> {rest}
                  </li>
                </Rise>
              ))}
            </ul>
          </div>
          <Ask at={ask2}>Avisa no grupo que eu levo as bebidas</Ask>
          <Tool at={sent - 8} done={sent + 8} busy="Enviando para Família…" label="Enviou uma mensagem para Família" />
          <Rise at={sent + 14} distance={10} frames={10} style={{ fontSize: 25, lineHeight: 1.45 }}>
            Pronto! Avisei no grupo que você leva as bebidas.
          </Rise>
        </div>
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 12,
            margin: "6px 20px 20px",
            padding: "12px 12px 12px 22px",
            border: `1px solid ${C.aiLine}`,
            borderRadius: 22,
            background: "#fff",
            fontSize: 23,
            color: composer ? C.aiInk : C.aiMuted,
            minHeight: 64,
          }}
        >
          <span>
            {composer || "Responder…"}
            {composer ? <span style={{ opacity: Math.floor(f / 8) % 2 ? 0 : 1 }}>|</span> : null}
          </span>
          <div
            style={{
              marginLeft: "auto",
              width: 42,
              height: 42,
              borderRadius: 12,
              background: C.aiInk,
              color: C.aiBg,
              display: "grid",
              placeItems: "center",
            }}
          >
            <svg width={22} height={22} viewBox="0 0 24 24">
              <path d="M12 19V5M6 11l6-6 6 6" fill="none" stroke="currentColor" strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </div>
        </div>
      </div>

      {/* What it can do, said one by one */}
      <div style={{ position: "absolute", left: 1040, top: 150, width: 780 }}>
        <Rise at={4} style={{ fontSize: 30, fontWeight: 700, color: C.onDeepSoft, letterSpacing: "0.01em" }}>
          A sua IA pode:
        </Rise>
        {FEATURES.map((ft, i) => {
          const t = pop(f, at(wordAt(ft.clip, ft.word)) - 3, 14);
          return (
            <div
              key={ft.label}
              style={{
                position: "absolute",
                top: 64 + i * 92,
                display: "flex",
                alignItems: "center",
                gap: 22,
                opacity: interpolate(t, [0, 0.3], [0, 1], { extrapolateRight: "clamp" }),
                transform: `translateX(${(1 - t) * 50}px)`,
              }}
            >
              <div
                style={{
                  width: 68,
                  height: 68,
                  borderRadius: 20,
                  background: "rgba(47,201,160,0.16)",
                  border: "1px solid rgba(47,201,160,0.35)",
                  color: C.mint,
                  display: "grid",
                  placeItems: "center",
                  transform: `scale(${lerp(0.6, 1, t)})`,
                }}
              >
                {icon(ft.d)}
              </div>
              <span style={{ fontSize: 38, fontWeight: 700, letterSpacing: "-0.015em", color: "#f6fcf9" }}>{ft.label}</span>
            </div>
          );
        })}
      </div>
    </AbsoluteFill>
  );
};
