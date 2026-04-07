import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Backend target: use VITE_API_TARGET env var or default to HTTP for local dev.
// For HTTPS backend: VITE_API_TARGET=https://localhost:8444 npm run dev
const apiTarget = process.env.VITE_API_TARGET || 'http://localhost:8444'

export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': {
        target: apiTarget,
        secure: false,
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
  },
})
