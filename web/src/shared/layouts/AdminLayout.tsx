import { ApartmentOutlined, BookOutlined, CalendarOutlined, DashboardOutlined, DatabaseOutlined, DownOutlined, IdcardOutlined, LockOutlined, LogoutOutlined, MenuFoldOutlined, MenuUnfoldOutlined, SafetyCertificateOutlined, UserOutlined, CheckSquareOutlined } from '@ant-design/icons'
import { Avatar, Button, Dropdown, Layout, Menu, Space, Tag, Typography } from 'antd'
import { useState } from 'react'
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../../app/providers/authContext'

const allItems = [
  { key: '/admin', icon: <DashboardOutlined />, label: <Link to="/admin">管理概览</Link> },
  { key: '/admin/registrations', icon: <SafetyCertificateOutlined />, label: <Link to="/admin/registrations">注册申请</Link> },
  { key: '/admin/users', icon: <IdcardOutlined />, label: <Link to="/admin/users">用户与角色</Link> },
  { key: '/admin/organization', icon: <ApartmentOutlined />, label: <Link to="/admin/organization">组织与业务</Link> },
  { key: '/admin/academic', icon: <CalendarOutlined />, label: <Link to="/admin/academic">学期与周次</Link> },
  { key: '/admin/master-data', icon: <DatabaseOutlined />, label: <Link to="/admin/master-data">基础数据</Link> },
  { key: '/admin/dictionaries', icon: <BookOutlined />, label: <Link to="/admin/dictionaries">字典管理</Link> },
]
const inspectionItems = [{ key: '/inspections/attendance', icon: <CheckSquareOutlined />, label: <Link to="/inspections/attendance">到课率检查</Link> }]

export function AdminLayout() {
  const [collapsed, setCollapsed] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  const { user, signOut } = useAuth()
	const inInspectionWorkspace = location.pathname.startsWith('/inspections')
	const accountRoleLabel = user?.isSuperAdmin ? '超级管理员' : user?.systemRole === 'ADMIN' ? '系统管理员' : '普通成员'
	const items = inInspectionWorkspace ? inspectionItems : user?.isSuperAdmin ? allItems : allItems.filter(item => ['/admin', '/admin/master-data', '/admin/dictionaries'].includes(item.key))
  const selectedKey = location.pathname === '/admin' ? '/admin' : items.find(item => item.key !== '/admin' && location.pathname.startsWith(item.key))?.key
	const accountItems = [
		...(!inInspectionWorkspace ? [
    { key: 'profile', icon: <UserOutlined />, label: '个人信息' },
    { key: 'security', icon: <LockOutlined />, label: '账户安全' },
    { type: 'divider' as const },
		] : []),
    { key: 'logout', icon: <LogoutOutlined />, label: '退出登录', danger: true },
  ]
  const onAccountAction = async ({ key }: { key: string }) => {
    if (key === 'logout') {
      await signOut()
      navigate('/login', { replace: true })
      return
    }
    navigate(`/admin/${key}`)
  }
  return <Layout className="admin-shell">
    <a className="skip-link" href="#main-content">跳到主要内容</a>
    <Layout.Sider
      width={248}
      id="admin-navigation"
      className="admin-sider"
      breakpoint="lg"
      collapsed={collapsed}
      collapsedWidth={80}
      onBreakpoint={(broken) => {
        if (broken) setCollapsed(true)
      }}
    >
      <div className="admin-brand">
        <span className="admin-brand-mark">计</span>
        <div className="admin-brand-copy">
			<strong>日常管理系统</strong>
			<small>{inInspectionWorkspace ? '日常检查业务' : user?.isSuperAdmin ? '超级管理员工作台' : '系统管理员工作台'}</small>
        </div>
      </div>
      <Menu mode="inline" theme="dark" selectedKeys={selectedKey ? [selectedKey] : []} items={items} />
      <div className="admin-sider-footer">
        <Button
          type="text"
          className="sider-toggle"
          icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
          aria-label={collapsed ? '展开侧边栏' : '收起侧边栏'}
          aria-controls="admin-navigation"
          aria-expanded={!collapsed}
          onClick={() => setCollapsed(value => !value)}
        />
      </div>
    </Layout.Sider>
    <Layout>
      <Layout.Header className="admin-header">
        <div className="admin-header-title"><Typography.Text className="admin-header-eyebrow">计算机学院</Typography.Text><Typography.Text strong>日常管理与基础平台</Typography.Text></div>
        <Dropdown menu={{ items: accountItems, onClick: onAccountAction }} placement="bottomRight"><Button type="text" className="account-trigger" aria-label={`${user?.name ?? '当前用户'}账户菜单`}><Space><Avatar size={34} icon={<UserOutlined />} /><span className="account-copy"><strong>{user?.name}</strong><small>{accountRoleLabel}</small></span><Tag color="blue">在线</Tag><DownOutlined /></Space></Button></Dropdown>
      </Layout.Header>
      <Layout.Content id="main-content" className="admin-content"><Outlet /></Layout.Content>
    </Layout>
  </Layout>
}
