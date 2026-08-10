import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: [
      {
        // The migrated testone sandbox imports itself through this scoped
        // alias so it never collides with ALemonX's own @/ imports.
        find: '@testone',
        replacement: fileURLToPath(new URL('./src/features/testone', import.meta.url))
      }
    ]
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:17390',
        ws: true
      }
    }
  },
  build: {
    outDir: '../dist',
    emptyOutDir: true,
    rolldownOptions: {
      output: {
        manualChunks(id) {
          if (
            id.includes('node_modules/react/') ||
            id.includes('node_modules/react-dom/') ||
            id.includes('node_modules/react-router-dom/')
          )
            return 'react'
          if (
            id.includes('node_modules/@reduxjs/') ||
            id.includes('node_modules/react-redux/') ||
            id.includes('node_modules/redux-persist/')
          )
            return 'state'
          if (id.includes('node_modules/markdown-to-jsx/')) return 'markdown'
          if (
            id.includes('node_modules/antd/') ||
            id.includes('node_modules/@ant-design/')
          )
            return 'antd'
          if (id.includes('node_modules/slate')) return 'slate'
          if (id.includes('node_modules/dayjs/')) return 'dayjs'
          if (
            id.includes('node_modules/flatted/') ||
            id.includes('node_modules/crypto-js/') ||
            id.includes('node_modules/lodash-es/')
          )
            return 'testone-vendor'
        }
      }
    }
  }
})
