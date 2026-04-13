import { defineConfig, externalizeDepsPlugin } from 'electron-vite'
import { resolve } from 'path'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  main: {
    plugins: [externalizeDepsPlugin()],
    build: {
      rollupOptions: {
        input: {
          index: resolve(__dirname, 'electron/main/index.ts')
        }
      }
    }
  },
  preload: {
    plugins: [externalizeDepsPlugin()],
    build: {
      rollupOptions: {
        input: {
          index: resolve(__dirname, 'electron/preload/index.ts')
        }
      }
    }
  },
  renderer: {
    root: resolve(__dirname, 'frontend'),
    build: {
      rollupOptions: {
        input: {
          index: resolve(__dirname, 'frontend/index.html')
        }
      }
    },
    plugins: [tailwindcss(), react()],
    resolve: {
      alias: {
        '@': resolve(__dirname, 'frontend/src')
      }
    },
    server: {
      port: 5173
    }
  }
})
