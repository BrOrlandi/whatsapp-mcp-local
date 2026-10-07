import React from "react";
import { AbsoluteFill, interpolate, useCurrentFrame } from "remotion";
import { C, ease, lerp, pop } from "../theme";
import { useScene } from "../timeline";

// "Mas atenção. O WhatsApp MCP não foi feito para disparar mensagens em massa.
// Isso vai contra as regras da Meta e pode bloquear o seu número. Então, nada
// de respostas automáticas ou da mesma mensagem para muitos contatos. Ele
// existe para levar as suas conversas até a sua IA. Use por sua conta e risco.
// Com um uso normal, no ritmo de uma pessoa, você não deve ter problemas."

const clamp = { extrapolateLeft: "clamp", extrapolateRight: "clamp" } as const;

const Mark: React.FC<{ ok: boolean; size?: number }> = ({ ok, size = 52 }) => (
  <div
    style={{
      width: size,
      height: size,
      flex: "none",
      borderRadius: size / 2,
      background: ok ? C.mint : "rgba(255,143,125,0.16)",
      border: ok ? "none" : `2px solid ${C.coral}`,
      color: ok ? C.mintInk : C.coral,
      display: "grid",
      placeItems: "center",
    }}
  >
    <svg width={size * 0.56} height={size * 0.56} viewBox="0 0 24 24">
      <path
        d={ok ? "M5 12.5l4.5 4.5L19 7.5" : "M6.5 6.5l11 11M17.5 6.5l-11 11"}
        fill="none"
        stroke="currentColor"
        strokeWidth={3}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  </div>
);

const Line: React.FC<{ at: number; ok: boolean; children: React.ReactNode; out?: number }> = ({ at, ok, children, out = Infinity }) => {
  const f = useCurrentFrame();
  const t = pop(f, at, 15);
  const gone = ease(f, out, 10);
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 24,
        fontSize: 44,
        fontWeight: 700,
        letterSpacing: "-0.02em",
        color: ok ? "#f6fcf9" : "#f3dcd6",
        opacity: interpolate(t, [0, 0.3], [0, 1], clamp) * (1 - gone),
        transform: `translateX(${(1 - t) * 40}px)`,
      }}
    >
      <Mark ok={ok} />
      {children}
    </div>
  );
};

export const Warning: React.FC = () => {
  const f = useCurrentFrame();
  const s = useScene("warning");

  const attention = s.cue("warning.sign");
  const p15 = s.cue("warning.head");
  const p16 = s.cue("warning.list");
  const p17 = s.cue("warning.risk");

  const sign = pop(f, attention, 10);
  const up = ease(f, p15 - 6, 22);
  const signSize = lerp(260, 150, up);
  const signY = lerp(300, 170, up);
  const ring = (offset: number) => {
    const p = ((f - attention - offset) % 45) / 45;
    return f > attention + offset && f < p15 + 20 ? p : -1;
  };

  const headA = ease(f, p15 + 2, 16) * (1 - ease(f, p17 - 4, 10));
  const headB = ease(f, p17, 16);
  const pills = 1 - ease(f, p16 - 4, 10);

  return (
    <AbsoluteFill style={{ alignItems: "center" }}>
      {/* The sign */}
      <div style={{ position: "absolute", left: 960 - signSize / 2, top: signY - signSize / 2, width: signSize, height: signSize }}>
        {[0, 15, 30].map((o) => {
          const p = ring(o);
          return p >= 0 ? (
            <div
              key={o}
              style={{
                position: "absolute",
                inset: 0,
                borderRadius: "50%",
                border: `4px solid ${C.amber}`,
                transform: `scale(${0.6 + p * 1.2})`,
                opacity: (1 - p) * 0.5,
              }}
            />
          ) : null;
        })}
        <svg viewBox="0 0 100 100" width={signSize} height={signSize} style={{ transform: `scale(${sign})`, filter: "drop-shadow(0 20px 40px rgba(246,185,74,0.35))" }}>
          <path d="M50 8 L94 86 Q96 92 89 92 L11 92 Q4 92 6 86 Z" fill={C.amber} stroke={C.amber} strokeWidth={6} strokeLinejoin="round" />
          <rect x={45} y={34} width={10} height={32} rx={5} fill="#2a1d05" />
          <circle cx={50} cy={78} r={6} fill="#2a1d05" />
        </svg>
      </div>

      {/* "Não é para disparo em massa" */}
      <div
        style={{
          position: "absolute",
          top: 290,
          fontSize: 76,
          fontWeight: 800,
          letterSpacing: "-0.035em",
          color: "#fff7ea",
          opacity: headA,
          transform: `translateY(${(1 - ease(f, p15 + 2, 16)) * 24}px)`,
        }}
      >
        Não é para disparo em massa
      </div>
      <div style={{ position: "absolute", top: 440, display: "flex", gap: 28, opacity: pills }}>
        {[
          { cue: "warning.rules", text: "Vai contra as regras da Meta" },
          { cue: "warning.block", text: "Pode bloquear o seu número" },
        ].map((p) => {
          const t = pop(f, s.cue(p.cue), 14);
          return (
            <div
              key={p.cue}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 16,
                padding: "18px 32px 18px 22px",
                borderRadius: 999,
                background: "rgba(255,143,125,0.12)",
                border: `2px solid rgba(255,143,125,0.55)`,
                fontSize: 38,
                fontWeight: 700,
                color: "#ffd9d1",
                opacity: interpolate(t, [0, 0.3], [0, 1], clamp),
                transform: `scale(${lerp(0.7, 1, t)})`,
              }}
            >
              <Mark ok={false} size={44} />
              {p.text}
            </div>
          );
        })}
      </div>

      <div style={{ position: "absolute", top: 440, left: 420, display: "flex", flexDirection: "column", gap: 34 }}>
        <Line at={s.cue("warning.auto")} ok={false} out={p17 - 4}>
          Respostas automáticas
        </Line>
        <Line at={s.cue("warning.same")} ok={false} out={p17 - 4}>
          A mesma mensagem para muitos contatos
        </Line>
        {s.has("warning.purpose") ? (
          <Line at={s.cue("warning.purpose")} ok out={p17 - 4}>
            Levar as suas conversas até a sua IA
          </Line>
        ) : null}
      </div>

      {/* "Use por sua conta e risco" */}
      <div
        style={{
          position: "absolute",
          top: 290,
          fontSize: 76,
          fontWeight: 800,
          letterSpacing: "-0.035em",
          color: "#fff7ea",
          opacity: headB,
          transform: `translateY(${(1 - headB) * 24}px)`,
        }}
      >
        Use por sua conta e risco
      </div>
      {(() => {
        const t = pop(f, s.cue("warning.normal"), 14);
        return (
          <div
            style={{
              position: "absolute",
              top: 450,
              display: "flex",
              alignItems: "center",
              gap: 26,
              padding: "30px 44px 30px 32px",
              borderRadius: 32,
              background: "rgba(47,201,160,0.12)",
              border: "2px solid rgba(47,201,160,0.45)",
              opacity: interpolate(t, [0, 0.3], [0, 1], clamp),
              transform: `scale(${lerp(0.8, 1, t)})`,
            }}
          >
            <Mark ok size={64} />
            <div>
              <div style={{ fontSize: 46, fontWeight: 800, letterSpacing: "-0.02em", color: "#f6fcf9" }}>
                Uso normal, no ritmo de uma pessoa
              </div>
              <div style={{ marginTop: 6, fontSize: 32, fontWeight: 500, color: C.onDeepSoft }}>
                assim, você não deve ter problemas
              </div>
            </div>
          </div>
        );
      })()}
    </AbsoluteFill>
  );
};
