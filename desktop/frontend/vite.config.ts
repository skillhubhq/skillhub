import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => ({
  plugins: [react()],
  base: "./",
  build: {
    target: "es2022",
    ...(mode === "web"
      ? { outDir: "../../internal/webui/static/dist", emptyOutDir: true }
      : {}),
  },
  define: {
    __NATIVE_SMOKE__: JSON.stringify(mode === "native-smoke"),
    __WEB__: JSON.stringify(mode === "web"),
  },
  resolve: { dedupe: ["@bufbuild/protobuf", "@connectrpc/connect"] },
}));
