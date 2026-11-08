import react from "@vitejs/plugin-react";
import { resolve } from "node:path";
import { defineConfig, type Plugin } from "vite";
import { renderSite } from "./site.ts";

const siteRoot = "/mock-site";

const mockSite = (): Plugin => ({
  name: "pageroom-mock-site",
  configureServer: (server) => {
    server.middlewares.use((request, response, next) => {
      if (!request.url?.startsWith(siteRoot)) return next();
      response.setHeader("Content-Type", "text/html");
      response.end(renderSite(request.url.slice(siteRoot.length) || "/"));
    });
  },
});

export default defineConfig({
  root: resolve(import.meta.dirname, ".."),
  plugins: [react(), mockSite()],
  resolve: {
    alias: [{ find: /^.*\/internal\/app\/app$/, replacement: resolve(import.meta.dirname, "mock.ts") }],
  },
  server: { port: 0 },
});
