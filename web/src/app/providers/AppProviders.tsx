import { App as AntDesignApp, ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter } from 'react-router-dom'
import type { ReactNode } from 'react'
import { AuthProvider } from './AuthProvider'

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } } })

export function AppProviders({ children }: { children: ReactNode }) {
  return <ConfigProvider theme={{ token: { colorPrimary: '#0b5cad', borderRadius: 8, fontFamily: '"Noto Sans SC","PingFang SC","Microsoft YaHei",sans-serif' } }}><AntDesignApp><QueryClientProvider client={queryClient}><BrowserRouter><AuthProvider>{children}</AuthProvider></BrowserRouter></QueryClientProvider></AntDesignApp></ConfigProvider>
}
