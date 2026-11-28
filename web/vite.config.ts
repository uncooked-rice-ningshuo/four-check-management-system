import { Script } from 'node:vm'
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import type { Plugin } from 'vite'

// WebVPN executes the entry as a classic script after rewriting the response.
function webvpnEntry(): Plugin {
  return {
    name: 'webvpn-classic-entry',
    apply: 'build',
    transformIndexHtml: {
      order: 'post',
      handler(html, { bundle }) {
        const entry = Object.values(bundle ?? {}).find(chunk => chunk.type === 'chunk' && chunk.isEntry)
        if (!entry || entry.type !== 'chunk') throw new Error('Missing WebVPN entry chunk')
        new Script(entry.code, { filename: entry.fileName })
        const moduleTag = `<script type="module" crossorigin src="/${entry.fileName}"></script>`
        if (!html.includes(moduleTag)) throw new Error('Missing WebVPN entry script tag')
        return html.replace(moduleTag, `<script defer src="/${entry.fileName}"></script>`)
      },
    },
  }
}

export default defineConfig(({ mode }) => ({
  plugins: [react(), mode === 'webvpn' && webvpnEntry()],
  build: mode === 'webvpn' ? {
    modulePreload: false,
    cssCodeSplit: false,
    rollupOptions: { output: { format: 'iife', inlineDynamicImports: true } },
  } : undefined,
  server: { proxy: { '/api': 'http://localhost:8080' } },
  test: { environment: 'jsdom', setupFiles: ['./src/test/setup.ts'], testTimeout: 10_000, fileParallelism: false, exclude: ['**/node_modules/**', '**/.git/**', 'midscene/**'] },
}))
