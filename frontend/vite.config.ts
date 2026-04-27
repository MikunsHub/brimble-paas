import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    allowedHosts: [
      // TODO: remove
      'e5bd-2c0f-f5c0-b10-4451-a583-3672-901f-202e.ngrok-free.app',
    ],
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/s3-upload': {
        target: 'http://localhost:4566',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/s3-upload/, ''),
      },
    },
  },
})
