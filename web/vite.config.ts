import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// WSL2에서 저장소가 /mnt/c 같은 Windows 파일시스템에 있으면 inotify 이벤트가
// 리눅스 쪽으로 전파되지 않아 HMR이 파일 변경을 놓친다. 그때만 폴링으로 넘긴다.
// 네이티브 파일시스템에서는 폴링이 불필요한 CPU를 쓰므로 켜지 않는다.
const onWindowsMount = process.cwd().startsWith("/mnt/");

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    watch: onWindowsMount ? { usePolling: true, interval: 400 } : undefined,
    proxy: {
      "/api": "http://localhost:8080",
      "/mcp": "http://localhost:8080",
      "/openapi.json": "http://localhost:8080",
    },
  },
  build: { target: "es2022", sourcemap: false },
});
