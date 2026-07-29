import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  define: {
    // isomorphic-git touches `process` in a few code paths
    "process.env": {},
  },
  resolve: {
    alias: {
      buffer: "buffer",
    },
  },
});
