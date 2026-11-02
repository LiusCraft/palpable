import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': path.resolve(import.meta.dirname, 'src') } },
  server: {
    port: 5174,
    // 开发期把 /api 代理到本地的 palpable-admin（默认 :8090），前端代码只写相对地址
    proxy: { '/api': { target: 'http://localhost:8090' } },
  },
})
