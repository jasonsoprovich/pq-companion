import { defineConfig, externalizeDepsPlugin } from 'electron-vite'
import { resolve } from 'path'
import { readdirSync } from 'fs'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// Fails a production build when docs/help has no articles, so a moved or
// mis-globbed help folder can't ship an empty Help page.
function helpDocsCheck() {
  return {
    name: 'pq-help-docs-check',
    apply: 'build' as const,
    buildStart() {
      const dir = resolve(__dirname, 'docs/help')
      let n = 0
      try {
        n = readdirSync(dir).filter((f) => f.endsWith('.md')).length
      } catch {
        n = 0
      }
      if (n === 0) throw new Error(`no help articles found in ${dir}`)
    },
  }
}

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
    plugins: [tailwindcss(), react(), helpDocsCheck()],
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
