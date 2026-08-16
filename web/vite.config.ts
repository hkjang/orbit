import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:8080",
      "/mcp": "http://localhost:8080",
      "/openapi.json": "http://localhost:8080",
    },
  },
  build: { target: "es2022", sourcemap: false },
});
