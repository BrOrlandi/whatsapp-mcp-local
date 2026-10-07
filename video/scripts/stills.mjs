import { bundle } from "@remotion/bundler";
import { renderStill, selectComposition } from "@remotion/renderer";
import path from "node:path";
const root = process.argv[2];
const outDir = process.argv[3];
const frames = process.argv.slice(4).map(Number);
const serveUrl = await bundle({ entryPoint: path.join(root, "src/index.ts"), publicDir: path.join(root, "public") });
const composition = await selectComposition({ serveUrl, id: "Explainer" });
for (const frame of frames) {
  await renderStill({ serveUrl, composition, frame, output: path.join(outDir, `f${String(frame).padStart(4, "0")}.jpg`), imageFormat: "jpeg", jpegQuality: 80, scale: 0.5 });
  console.log("frame", frame);
}
