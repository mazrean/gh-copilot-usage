import { defineConfig } from "vite";
import UnoCSS from "@unocss/vite";

export default defineConfig({
  plugins: [UnoCSS()],
  build: {
    outDir: "../internal/server/web",
    emptyOutDir: true,
    assetsDir: ".",
    rollupOptions: {
      input: "src/main.ts",
      output: {
        entryFileNames: "app.js",
        chunkFileNames: "app-[name].js",
        assetFileNames: "app[extname]",
      },
    },
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8765",
    },
  },
});
