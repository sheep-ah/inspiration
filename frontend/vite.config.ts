import { defineConfig } from "vite";

// 构建产物使用固定文件名，便于提交到仓库并让 CI 校验"产物是否与源码一致"。
// 这样新克隆仓库无需 Node 也能直接 go build（//go:embed all:frontend/dist）。
export default defineConfig({
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "es2022",
    sourcemap: false,
    rollupOptions: {
      output: {
        entryFileNames: "assets/index.js",
        chunkFileNames: "assets/[name].js",
        assetFileNames: "assets/index.[ext]",
      },
    },
  },
});
