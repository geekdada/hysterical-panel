import { readFileSync } from "node:fs";
import { paraglideVitePlugin } from "@inlang/paraglide-js";
import { defineConfig } from "vite";
import { tanstackStart } from "@tanstack/react-start/plugin/vite";
import viteReact from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { nitro } from "nitro/vite";

const appVersion = readFileSync(new URL("../VERSION", import.meta.url), "utf8").trim();

export default defineConfig({
  server: {
    port: 3000,
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
});
