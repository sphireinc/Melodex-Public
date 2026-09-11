import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  base: "./",
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    // Video.js is intentionally isolated into a lazy media chunk. Keep the
    // warning budget focused on the initial app/vendor chunks rather than
    // flagging that known, separately loaded dependency on every build.
    chunkSizeWarningLimit: 768,
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            {
              name: "video",
              test: /node_modules[\\/]video\.js/,
              priority: 2,
            },
            {
              name: "vendor",
              test: /node_modules[\\/]/,
            },
          ],
        },
      },
    },
  },
});
