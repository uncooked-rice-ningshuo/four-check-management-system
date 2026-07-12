import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
export default defineConfig({publicDir:'../页面使用素材',plugins:[react()],server:{proxy:{'/api':'http://localhost:8080'}},test:{environment:'jsdom',setupFiles:'./src/test/setup.ts'}})
