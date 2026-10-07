import React from "react";
import { AbsoluteFill, interpolate, staticFile, useCurrentFrame } from "remotion";
import { AppWindow, Shot, StepTitle } from "../components";
import { C, ease, lerp, pop } from "../theme";
import { useScene } from "../timeline";

// "Ao abrir, ele mostra um QR code. No celular, abra o WhatsApp, entre em
// Dispositivos conectados e aponte a câmera para o código." — "Pronto: o seu
// WhatsApp está conectado."

const WIN = { x: 110, y: 200, w: 1080, h: 710 };
const K = WIN.w / 1000; // window pixels per CSS pixel of the screenshot

/** A box over a region of the screenshot, given in its CSS pixels. */
const Highlight: React.FC<{ x: number; y: number; w: number; h: number; t: number }> = ({ x, y, w, h, t }) =>
  t > 0 ? (
    <div
      style={{
        position: "absolute",
        left: x * K - 8,
        top: y * K - 8,
        width: w * K + 16,
        height: h * K + 16,
        borderRadius: 16,
        border: `4px solid ${C.mint}`,
        boxShadow: `0 0 0 9999px rgba(3, 14, 12, ${0.45 * t}), 0 0 40px rgba(47,201,160,0.5)`,
        opacity: t,
        transform: `scale(${lerp(1.06, 1, t)})`,
      }}
    />
  ) : null;

const PHONE = { x: 1360, y: 190, w: 340, h: 690 };
// The white box around the QR code (259 CSS pixels) fills the 200-pixel viewfinder.
const QR_K = 200 / 259;

export const Qr: React.FC = () => {
  const f = useCurrentFrame();
  const s = useScene("qr");

  const winIn = pop(f, 2, 16);
  const qrWord = s.cue("qr.qr");
  const phoneWord = s.cue("qr.phone");
  const devices = s.cue("qr.devices");
  const aim = s.cue("qr.aim");
  const ok = s.cue("qr.ok");

  const hlQr = Math.max(
    ease(f, qrWord - 2, 10) * (1 - ease(f, phoneWord + 6, 10)),
    ease(f, aim, 10) * (1 - ease(f, ok, 8)),
  );
  const hlStep = ease(f, devices - 2, 10) * (1 - ease(f, aim - 4, 8));
  const connected = ease(f, ok, 10);
  const hlCard = ease(f, ok + 12, 12);

  const phoneIn = pop(f, phoneWord, 16);
  const tilt = ease(f, aim - 4, 18);
  const scanning = f >= aim + 8 && f < ok;
  const scanY = (Math.sin((f - aim) / 7) + 1) / 2;
  const success = pop(f, ok, 12);

  return (
    <AbsoluteFill>
      <StepTitle step={2} title="Conecte o seu WhatsApp" x={WIN.x} y={44} />
      <AppWindow
        x={WIN.x}
        y={WIN.y}
        width={WIN.w}
        height={WIN.h}
        style={{ opacity: winIn, transform: `translateY(${(1 - winIn) * 50}px)` }}
      >
        <Shot src="01-qr.png" width={WIN.w} />
        <Shot src="02-escolher-full.png" width={WIN.w} opacity={connected} />
        <Highlight x={370} y={361} w={259} h={258} t={hlQr} />
        <Highlight x={262} y={274} w={470} h={48} t={hlStep} />
        <Highlight x={190} y={128} w={620} h={148} t={hlCard} />
      </AppWindow>

      {/* The phone, reading the code */}
      <div
        style={{
          position: "absolute",
          left: PHONE.x,
          top: PHONE.y,
          width: PHONE.w,
          height: PHONE.h,
          opacity: interpolate(phoneIn, [0, 0.3], [0, 1], { extrapolateRight: "clamp" }),
          transform: `translateX(${(1 - phoneIn) * 260 - tilt * 70}px) rotate(${-tilt * 7}deg)`,
          transformOrigin: "50% 80%",
        }}
      >
        <div
          style={{
            position: "absolute",
            inset: 0,
            borderRadius: 56,
            background: "#0d0f10",
            border: "2px solid #2d3533",
            boxShadow: "0 50px 100px -30px rgba(0,0,0,0.75)",
            padding: 12,
          }}
        >
          <div
            style={{
              position: "relative",
              height: "100%",
              borderRadius: 44,
              overflow: "hidden",
              background: "#0b141a",
              fontFamily: "-apple-system, BlinkMacSystemFont, 'Helvetica Neue', sans-serif",
              color: "#e9edef",
            }}
          >
            <div style={{ height: 48, display: "flex", alignItems: "center", justifyContent: "space-between", padding: "0 28px", fontSize: 17, fontWeight: 600 }}>
              <span>9:41</span>
              <span style={{ width: 90, height: 26, borderRadius: 13, background: "#000", position: "absolute", left: "50%", marginLeft: -45, top: 10 }} />
              <span style={{ display: "flex", gap: 5 }}>
                <span style={{ width: 18, height: 11, borderRadius: 3, border: "1.5px solid #e9edef" }} />
              </span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 12, padding: "8px 18px 16px", fontSize: 21, fontWeight: 600 }}>
              <svg width={22} height={22} viewBox="0 0 24 24">
                <path d="M15 5l-7 7 7 7" fill="none" stroke="#e9edef" strokeWidth={2.4} strokeLinecap="round" strokeLinejoin="round" />
              </svg>
              Conectar um dispositivo
            </div>
            {/* The camera */}
            <div
              style={{
                position: "absolute",
                left: 0,
                right: 0,
                top: 112,
                bottom: 0,
                background: "radial-gradient(circle at 50% 40%, #26302e, #0c1210 75%)",
              }}
            >
              <div style={{ position: "absolute", left: 48, top: 70, width: 220, height: 220 }}>
                {/* What the camera sees: the code on the computer's screen */}
                <div
                  style={{
                    position: "absolute",
                    inset: 10,
                    borderRadius: 6,
                    backgroundColor: "#fff",
                    backgroundImage: `url(${staticFile("shots/01-qr.png")})`,
                    backgroundSize: `${1000 * QR_K}px auto`,
                    backgroundPosition: `${-370 * QR_K}px ${-361 * QR_K}px`,
                    opacity: interpolate(tilt, [0.4, 1], [0, 0.95], { extrapolateLeft: "clamp", extrapolateRight: "clamp" }) * (1 - success),
                    filter: `blur(${(1 - tilt) * 4}px)`,
                  }}
                />
                {[0, 1, 2, 3].map((c) => (
                  <div
                    key={c}
                    style={{
                      position: "absolute",
                      width: 46,
                      height: 46,
                      left: c % 2 ? undefined : 0,
                      right: c % 2 ? 0 : undefined,
                      top: c < 2 ? 0 : undefined,
                      bottom: c < 2 ? undefined : 0,
                      borderColor: C.mint,
                      borderStyle: "solid",
                      borderWidth: `${c < 2 ? 5 : 0}px ${c % 2 ? 5 : 0}px ${c < 2 ? 0 : 5}px ${c % 2 ? 0 : 5}px`,
                      borderRadius: 10,
                    }}
                  />
                ))}
                {scanning ? (
                  <div
                    style={{
                      position: "absolute",
                      left: 8,
                      right: 8,
                      top: 10 + scanY * 196,
                      height: 4,
                      borderRadius: 2,
                      background: C.mint,
                      boxShadow: `0 0 18px 4px rgba(47,201,160,0.7)`,
                    }}
                  />
                ) : null}
                {success > 0 ? (
                  <div
                    style={{
                      position: "absolute",
                      inset: 30,
                      borderRadius: "50%",
                      background: C.mint,
                      display: "grid",
                      placeItems: "center",
                      transform: `scale(${success})`,
                    }}
                  >
                    <svg width={90} height={90} viewBox="0 0 24 24">
                      <path d="M5 12.5l4.5 4.5L19 7.5" fill="none" stroke={C.mintInk} strokeWidth={2.8} strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  </div>
                ) : null}
              </div>
              <div style={{ position: "absolute", left: 30, right: 30, top: 330, textAlign: "center", fontSize: 18, lineHeight: 1.4, color: "#c4cecb" }}>
                {success > 0.5 ? "Dispositivo conectado" : "Aponte a câmera para o QR code na tela do computador"}
              </div>
            </div>
          </div>
        </div>
      </div>
    </AbsoluteFill>
  );
};
