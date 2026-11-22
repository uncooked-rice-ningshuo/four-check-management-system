import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { ConfigProvider, Layout } from 'antd'
import { BrowserRouter } from 'react-router-dom'
import { NotificationBell } from '../src/features/notifications/NotificationBell'
import '../src/shared/styles/global.css'
import './harness.css'

// 直接渲染而非定义组件，避免与证据目录无关的 Fast Refresh 警告。
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ConfigProvider theme={{ token: { colorPrimary: '#2563eb' } }}>
      <BrowserRouter>
        <Layout className="admin-shell">
          <Layout>
            <Layout.Header className="admin-header" style={{ justifyContent: 'flex-end' }}>
              <NotificationBell />
            </Layout.Header>
            <Layout.Content className="admin-content">
              <div className="admin-page">铃铛面板隔离验证（数据来自 URL mode 参数，不访问真实接口）</div>
            </Layout.Content>
          </Layout>
        </Layout>
      </BrowserRouter>
    </ConfigProvider>
  </StrictMode>,
)
