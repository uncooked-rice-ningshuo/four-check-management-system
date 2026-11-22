import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const here = fileURLToPath(new URL('.', import.meta.url))

export default defineConfig({
  root: here,
  plugins: [react()],
  resolve: {
    alias: [
      { find: /^\.\/useInbox$/, replacement: `${here}useInbox.stub.ts` },
    ],
  },
  server: { host: '127.0.0.1', port: 5199, strictPort: true },
})
