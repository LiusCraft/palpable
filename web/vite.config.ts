import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    // 开发期把 /ws、/api、/world 代理到本地后端，前端代码只写相对地址
    proxy: {
      '/ws': { target: 'ws://localhost:8080', ws: true },
      '/world': { target: 'ws://localhost:8080', ws: true },
      '/api': { target: 'http://localhost:8080' },
    },
  },
})
