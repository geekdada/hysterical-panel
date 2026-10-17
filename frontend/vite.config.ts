import { readFileSync } from "node:fs";
import { paraglideVitePlugin } from "@inlang/paraglide-js";
import { defineConfig, loadEnv } from "vite";
import { tanstackStart } from "@tanstack/react-start/plugin/vite";
import viteReact from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { nitro } from "nitro/vite";

const appVersion = readFileSync(new URL("../VERSION", import.meta.url), "utf8").trim();

export default defineConfig(({ mode }) => ({
  server: {
    port: 3000,
    proxy: {
      // Nitro's dev middleware treats Sec-Fetch-Dest: image requests as static
      // assets and never reaches the /l/:id/favicon route, so dev proxies it.
      "^/l/[^/]+/favicon": {
        target: loadEnv(mode, process.cwd(), "").VITE_API_BASE_URL || "http://localhost:8090",
        changeOrigin: true,
        rewrite: (p) => p.replace(/^\/l\/([^/]+)\/favicon/, "/api/resource-links/$1/favicon"),
      },
    },
  },
  define: {
    __APP_VERSION__: JSON.stringify(appVersion),
  },
  resolve: {
    tsconfigPaths: true,
  },
  ssr: {
    // Force bundling so CJS package root isn't loaded by the SSR module runner.
    noExternal: ["@gravity-ui/icons"],
  },
  plugins: [
    paraglideVitePlugin({
      project: "./project.inlang",
      outdir: "./src/paraglide",
      outputStructure: "message-modules",
      cookieName: "hp:locale",
      strategy: ["cookie", "preferredLanguage", "baseLocale"],
      emitTsDeclarations: true,
    }),
    tanstackStart(),
    nitro(),
    viteReact(),
    tailwindcss(),
  ],
}));
