import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import { writeFileSync } from "node:fs";
import { resolve } from "node:path";

// A strict Content-Security-Policy for the shipped app. The live preview
// (Hugo on 127.0.0.1) is the only thing allowed in a frame, and no remote
// scripts can ever load into the editor. Dev mode skips it because Vite's
// hot reload needs inline scripts.
const csp = [
  "default-src 'self' wails: http://wails.localhost",
  // https: lets posts show images still hosted elsewhere (e.g. an old WordPress site)
  "img-src 'self' data: blob: https: wails: http://wails.localhost",
  "frame-src http://127.0.0.1:*",
  "style-src 'self' 'unsafe-inline'",
  "script-src 'self' wails: http://wails.localhost",
  "connect-src 'self' wails: http://wails.localhost",
  "font-src 'self' data:",
  "object-src 'none'",
  "base-uri 'none'",
].join("; ");

const cspPlugin = (): Plugin => ({
  name: "pageroom-csp",
  apply: "build",
  transformIndexHtml: (html) =>
    html.replace("<head>", `<head>\n    <meta http-equiv="Content-Security-Policy" content="${csp}" />`),
});

// The Go binary embeds frontend/dist, so it must exist even before the
// first frontend build (fresh clones, `go test ./...`).
const keepDist = (): Plugin => ({
  name: "pageroom-keep-dist",
  apply: "build",
  closeBundle: () => writeFileSync(resolve(__dirname, "dist/.gitkeep"), ""),
});

export default defineConfig({
  plugins: [react(), cspPlugin(), keepDist()],
  build: { target: "es2022", chunkSizeWarningLimit: 3000 },
  test: { environment: "jsdom" },
} as any);
