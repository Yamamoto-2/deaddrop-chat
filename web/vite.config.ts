import { defineConfig } from "vite";

// During `npm run dev`, proxy the WebSocket to the Go backend on :8080 so the
// frontend can be served by Vite's HMR server while still talking to the relay.
export default defineConfig({
  server: {
    port: 5173,
    proxy: {
      "/ws": {
        target: "ws://localhost:8080",
        ws: true,
      },
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
