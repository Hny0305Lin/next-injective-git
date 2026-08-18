import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

const root = fileURLToPath(new URL(".", import.meta.url));

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  define: {
    // isomorphic-git touches `process` in a few code paths
    "process.env": {},
  },
  resolve: {
    alias: {
      "@": path.resolve(root, "./src"),
      buffer: "buffer",
    },
  },
});
