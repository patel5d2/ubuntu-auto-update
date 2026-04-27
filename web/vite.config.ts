import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  // Default to localhost for local dev; override with VITE_PROXY_TARGET=http://backend:8080 in Docker.
  const target = env.VITE_PROXY_TARGET || 'http://localhost:8080'

  return {
    plugins: [react()],
    server: {
      host: '0.0.0.0',
      proxy: {
        '/api': {
          target,
          changeOrigin: true,
          ws: true,
        },
      },
    },
  }
})
