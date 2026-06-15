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
    // Pre-bundle the heaviest deps deterministically so esbuild does the work
    // once at startup instead of re-optimizing mid-session (which forces a full
    // page reload). lucide-react is named-imported across ~90 files; pinning it
    // here keeps the dep scanner from re-crawling that surface on cold start.
    optimizeDeps: {
      include: ['@xyflow/react', 'dagre', 'lucide-react', 'react-router-dom']
    },
    server: {
      port: 5173
    }
  }
})
