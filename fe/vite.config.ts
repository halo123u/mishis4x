import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8091",
        changeOrigin: true,
        secure: false,
        // ws: true - without it, ModelDisplay.tsx's WebSocket connection
        // to /api/models/display/ws gets treated as a plain HTTP request
        // by the dev proxy and never actually upgrades, so local dev
        // needs this explicitly (Vite doesn't infer it from the request
        // itself).
        ws: true,
      },
    },
  },
  build: {
    emptyOutDir: true,
  },
});
