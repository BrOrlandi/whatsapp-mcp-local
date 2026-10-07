import React from "react";
import { AbsoluteFill, useCurrentFrame } from "remotion";
import { BrandMark, Rise } from "../components";
import { C, ease, pop } from "../theme";
import { scene, wordAt } from "../timeline";

// "Conheça o WhatsApp MCP: o seu WhatsApp conectado à sua IA, direto do seu
// computador."

export const Brand: React.FC = () => {
  const f = useCurrentFrame();
  const s = scene("brand");
  const at = (abs: number) => abs - s.from;
  const mark = pop(f, 2, 16);
  const glow = ease(f, 0, 30);
  return (
    <AbsoluteFill style={{ alignItems: "center" }}>
      <div
        style={{
          position: "absolute",
          left: 960 - 420,
          top: 300 - 420,
          width: 840,
          height: 840,
          borderRadius: "50%",
          background: "radial-gradient(circle, rgba(47,201,160,0.25), rgba(47,201,160,0) 62%)",
          opacity: glow,
        }}
      />
      <div style={{ position: "absolute", top: 150, transform: `scale(${0.7 + mark * 0.3})` }}>
        <BrandMark size={210} draw={ease(f, 2, 34)} />
      </div>
      <Rise at={12} distance={40} frames={22} style={{ position: "absolute", top: 400 }}>
        <div style={{ fontSize: 132, fontWeight: 800, letterSpacing: "-0.045em", lineHeight: 1, color: "#f6fcf9" }}>
          WhatsApp MCP
        </div>
      </Rise>
      <div style={{ position: "absolute", top: 580, textAlign: "center", fontSize: 46, fontWeight: 500, lineHeight: 1.35, color: C.onDeepSoft }}>
        <Rise at={at(wordAt("05", "seu")) - 4}>
          O seu WhatsApp conectado à <span style={{ color: C.mint, fontWeight: 700 }}>sua IA</span>,
        </Rise>
        <Rise at={at(wordAt("05", "direto")) - 4}>direto do seu computador.</Rise>
      </div>
    </AbsoluteFill>
  );
};
