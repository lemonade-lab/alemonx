import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath, URL } from 'node:url'
import { brotliCompressSync, constants, gzipSync } from 'node:zlib'

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    {
      name: 'alemonx-compressed-assets',
      apply: 'build',
      generateBundle: {
        order: 'post',
        handler(_, bundle) {
          for (const [fileName, entry] of Object.entries(bundle)) {
            if (!fileName.startsWith('assets/') || !/\.(js|css)$/.test(fileName)) continue
            const source = entry.type === 'chunk' ? entry.code : entry.source
            if (source.length < 1024) continue
            const gzip = gzipSync(source)
            const brotli = brotliCompressSync(source, {
              params: { [constants.BROTLI_PARAM_QUALITY]: 6 }
            })
            this.emitFile({
              type: 'asset',
              fileName: `${fileName}.gz`,
              source: gzip
            })
            if (brotli.length < gzip.length)
              this.emitFile({
                type: 'asset',
                fileName: `${fileName}.br`,
                source: brotli
              })
          }
        }
      }
    }
  ],
  resolve: {
    alias: [
      {
        find: '@testone',
        replacement: fileURLToPath(new URL('./src/features/testone', import.meta.url))
      }
    ]
  },
  server: {
    // Keep test fixtures stable while local source files are being edited.
    hmr: process.env.ALEMONX_PLAYWRIGHT !== '1',
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
        codeSplitting: {
          groups: [{
            name: 'icons',
            test: /\/node_modules\/lucide-react\//,
            includeDependenciesRecursively: false
          }]
        }
      }
    }
  }
})
