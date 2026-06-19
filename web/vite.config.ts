import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Dev-прокси: фронт зовёт относительный /api/ask, Vite проксирует на Go-сервис.
// Так обходим возможное отсутствие CORS-заголовков у бэкенда.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8088',
        changeOrigin: true,
        rewrite: (p) => p.replace(/^\/api/, ''),
      },
    },
  },
})
