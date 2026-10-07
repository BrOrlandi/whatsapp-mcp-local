import curtoNarration from "./narration/curto.json";
import type { Clip, Cue, Version } from "./timeline";

// The cuts of the video. A cut is a script (narration/<id>.json), a pace, and
// the words each animation fires on; the scenes are the same for every cut.
// The first one, "longo" (2:34), was replaced by "curto" and lives in git
// history (commit 59d0b60).

const w = (clip: string, word: string, offset = 0, nth = 0): Cue => ({ clip, word, offset, nth });
const start = (clip: string, offset = 0): Cue => ({ clip, at: "start", offset });
const end = (clip: string, offset = 0): Cue => ({ clip, at: "end", offset });

/** The one-minute cut: a tighter script, the voice sped up, shorter scenes. */
export const curto: Version = {
  id: "curto",
  narration: curtoNarration as Clip[],
  lead: { ecosystem: 0.4, brand: 0.15, download: 0.15, qr: 0.15, choose: 0.15, features: 0.2, warning: 0.2, cta: 0.15 },
  tail: { ecosystem: 0.7, brand: 0.5, download: 0.8, qr: 1.0, choose: 0.5, features: 0.4, warning: 0.4, cta: 1.8 },
  pauses: { "03": 0.4, "10": 0.2, "11": 0.25 },
  pause: 0.15,
  enter: 10,
  out: 8,
  cues: {
    "tools.gather": w("01", "conecta", -6),
    "tools.gmail": w("01", "Gmail"),
    "tools.agenda": w("01", "agenda"),
    "tools.drive": w("01", "Drive"),
    "tools.slack": w("01", "Slack"),
    "tools.jira": w("01", "Jira"),
    "tools.github": w("01", "GitHub"),
    "tools.all": w("01", "GitHub", 10),
    "tools.dim": start("02"),
    "tools.alone": w("02", "WhatsApp", -6),
    "tools.named": w("02", "fora"),
    "tools.solved": w("03", "resolvido", -2),
    "brand.tag1": w("04", "MCP", 4),
    "brand.tag2": w("04", "MCP", 10),
    "download.mac": w("05", "Mac", -3),
    "download.windows": w("05", "Windows", -3),
    "download.linux": w("05", "Linux", -3),
    "download.click": w("05", "Linux", 6),
    "download.end": end("05"),
    "qr.qr": w("06", "QR", -2),
    "qr.phone": w("06", "WhatsApp", -6),
    "qr.devices": w("06", "Dispositivos", -2),
    "qr.aim": w("06", "conectados"),
    "qr.ok": end("06", 14),
    "choose.scroll": start("07", -6),
    "choose.pick": w("07", "IA"),
    "choose.add": w("07", "clique"),
    "choose.side": w("07", "Codex", -10),
    "choose.codex": w("07", "Codex", -4),
    "choose.cursor": w("07", "Cursor", -4),
    "choose.others": w("07", "outras", -4),
    "choose.paste": w("07", "texto", -4),
    "choose.copied": w("07", "colar"),
    "features.ask1": start("08", 10),
    "features.read": w("08", "ler"),
    "features.sum": w("08", "resumir", -8),
    "features.ask2": w("08", "responder", -2),
    "features.sent": w("08", "mandar"),
    "features.f0": w("08", "conversas", -3),
    "features.f1": w("08", "achar", -3),
    "features.f2": w("08", "resumir", -3),
    "features.f3": w("08", "responder", -3),
    "features.f4": w("08", "fotos", -3),
    "features.f5": w("08", "transcrever", -3),
    "features.f6": w("08", "enquetes", -3),
    "warning.sign": start("09", -6),
    "warning.head": w("09", "ele", -4),
    "warning.rules": w("09", "regras", -4),
    "warning.block": w("09", "bloquear", -4),
    "warning.list": start("10"),
    "warning.auto": w("10", "respostas", -4),
    "warning.same": w("10", "mesma", -4),
    "warning.risk": start("11"),
    "warning.normal": w("11", "ritmo", -4),
    "cta.free": w("12", "gratuito", -4),
    "cta.open": w("12", "codigo", -4),
    "cta.github": w("12", "GitHub", -4),
    "cta.download": w("12", "Baixe", -4),
    "cta.link": w("12", "link"),
  },
};

export const VERSIONS = [curto];
