import { ConfigProvider } from 'antd'
import { BrowserRouter } from 'react-router-dom'
import type { ReactNode } from 'react'
import { AuthProvider } from './AuthProvider'

export function AppProviders({ children }: { children: ReactNode }) {
  return <ConfigProvider theme={{ token: { colorPrimary: '#0869f7', borderRadius: 8, fontFamily: 'Inter,"PingFang SC","Microsoft YaHei",sans-serif' } }}><BrowserRouter><AuthProvider>{children}</AuthProvider></BrowserRouter></ConfigProvider>
}
