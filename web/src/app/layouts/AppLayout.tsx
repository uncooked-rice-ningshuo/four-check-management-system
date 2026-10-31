import {
  ApartmentOutlined,
  EnvironmentOutlined,
  FileTextOutlined,
  BuildOutlined,
  AppstoreOutlined,
  BookOutlined,
  CalendarOutlined,
  CheckSquareOutlined,
  ClearOutlined,
  DatabaseOutlined,
  DesktopOutlined,
  DownOutlined,
  HomeOutlined,
  IdcardOutlined,
  LockOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  MoonOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
  SunOutlined,
  TeamOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { Alert, Avatar, Button, Dropdown, Layout, Menu, Space, Tag, type MenuProps } from 'antd'
import { useState } from 'react'
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useAttendancePermissions } from '../../features/attendance/hooks/useAttendancePermissions'
import { hasNavigableInspectionAction } from '../../features/attendance/hooks/hasNavigableInspectionAction'
import { useAuth } from '../providers/authContext'
import { useInspectionContext } from '../../features/course-inspection/model/queries'
import { WorkContextBar } from '../../features/work-context/WorkContextBar'
import { useHygieneAreaTargets } from '../../features/hygiene-area/model/queries'

const allAdminItems = [
  { key: '/admin/registrations', icon: <SafetyCertificateOutlined />, label: <Link to="/admin/registrations">注册申请</Link> },
  { key: '/admin/users', icon: <IdcardOutlined />, label: <Link to="/admin/users">用户与角色</Link> },
  { key: '/admin/organization', icon: <ApartmentOutlined />, label: <Link to="/admin/organization">组织与业务</Link> },
  { key: '/admin/academic', icon: <CalendarOutlined />, label: <Link to="/admin/academic">学期与周次</Link> },
  { key: '/admin/master-data', icon: <DatabaseOutlined />, label: <Link to="/admin/master-data">基础数据</Link> },
  { key: '/admin/dictionaries', icon: <BookOutlined />, label: <Link to="/admin/dictionaries">字典管理</Link> },
]

const workbenchItem = { key: '/', icon: <HomeOutlined />, label: <Link to="/">工作台</Link> }

export function AppLayout() {
  const [collapsed, setCollapsed] = useState(false)
  const [signingOut, setSigningOut] = useState(false)
  const [logoutFailed, setLogoutFailed] = useState(false)
  const [openMenuState, setOpenMenuState] = useState<{ signature: string; keys: string[] }>({ signature: '', keys: [] })
  const navigate = useNavigate()
  const location = useLocation()
  const { user, signOut } = useAuth()
  const effectivePermissions = useAttendancePermissions()
  const hygieneTargets = useHygieneAreaTargets(!!user && !user.mustChangePassword)
  const accountRoleLabel = user?.primaryDepartmentName && user.organizationRoleName
    ? `${user.primaryDepartmentName} · ${user.organizationRoleName}`
    : user?.isSuperAdmin ? '超级管理员' : user?.systemRole === 'ADMIN' ? '系统管理员' : '普通成员'
  const attendanceContext = useInspectionContext({ typeCode: 'ATTENDANCE_RATE', purpose: 'view', kind: 'semesters' })
  const unattendedContext = useInspectionContext({ typeCode: 'UNATTENDED_CLASSROOM', purpose: 'view', kind: 'semesters' })
  const lateReturnContext = useInspectionContext({ typeCode: 'LATE_RETURN', purpose: 'view', kind: 'semesters' })
  const morningContext = useInspectionContext({ typeCode: 'MORNING_ATTENDANCE', purpose: 'view', kind: 'semesters' })
  const eveningSelfStudyContext = useInspectionContext({ typeCode: 'EVENING_SELF_STUDY', purpose: 'view', kind: 'semesters' })
  const dormitoryHygieneContext = useInspectionContext({ typeCode: 'DORMITORY_HYGIENE', purpose: 'view', kind: 'semesters' })
  const publicHygieneContext = useInspectionContext({ typeCode: 'PUBLIC_DORMITORY_HYGIENE', purpose: 'view', kind: 'semesters' })
  const classroomHygieneContext = useInspectionContext({ typeCode: 'CLASSROOM_HYGIENE', purpose: 'view', kind: 'semesters' })
  const canViewType = (id?: string) => user?.isSuperAdmin || (!!id && hasNavigableInspectionAction(effectivePermissions.data?.permissions, 'INSPECTION_VIEW', id))
  const adminItems = user?.isSuperAdmin
    ? allAdminItems
    : user?.systemRole === 'ADMIN'
      ? allAdminItems.filter(item => ['/admin/master-data', '/admin/dictionaries'].includes(item.key))
      : []
  const basicItems = adminItems.filter(item => ['/admin/registrations', '/admin/users'].includes(item.key))
  const configurationItems = [
    ...adminItems.filter(item => !['/admin/registrations', '/admin/users'].includes(item.key)),
    ...(hygieneTargets.data?.length ? [{ key: '/admin/hygiene-areas', icon: <ClearOutlined />, label: <Link to="/admin/hygiene-areas">卫生区域维护</Link> }] : []),
  ]
  const visibleInspectionItems = [
    ...(canViewType(attendanceContext.data?.inspectionTypeId) ? [{ key: '/inspections/attendance', icon: <TeamOutlined />, label: <Link to="/inspections/attendance">到课率检查</Link> }] : []),
    ...(canViewType(unattendedContext.data?.inspectionTypeId) ? [{ key: '/inspections/unattended-classroom', icon: <DesktopOutlined />, label: <Link to="/inspections/unattended-classroom">无机课堂检查</Link> }] : []),
    ...(canViewType(classroomHygieneContext.data?.inspectionTypeId) ? [{ key: '/inspections/classroom-hygiene', icon: <BuildOutlined />, label: <Link to="/inspections/classroom-hygiene">教室卫生</Link> }] : []),
    ...(canViewType(morningContext.data?.inspectionTypeId) ? [{ key: '/inspections/morning-attendance', icon: <SunOutlined />, label: <Link to="/inspections/morning-attendance">早锻炼/早签</Link> }] : []),
    ...(canViewType(eveningSelfStudyContext.data?.inspectionTypeId) ? [{ key: '/inspections/evening-self-study', icon: <MoonOutlined />, label: <Link to="/inspections/evening-self-study">晚自习</Link> }] : []),
    ...(canViewType(dormitoryHygieneContext.data?.inspectionTypeId) ? [{ key: '/inspections/dormitory-hygiene', icon: <ClearOutlined />, label: <Link to="/inspections/dormitory-hygiene">宿舍卫生</Link> }] : []),
    ...(canViewType(lateReturnContext.data?.inspectionTypeId) ? [{ key: '/inspections/late-return', icon: <MoonOutlined />, label: <Link to="/inspections/late-return">晚归</Link> }] : []),
    ...(canViewType(publicHygieneContext.data?.inspectionTypeId) ? [{ key: '/inspections/public-dormitory-hygiene', icon: <EnvironmentOutlined />, label: <Link to="/inspections/public-dormitory-hygiene">公共宿舍卫生</Link> }] : []),
  ]
  const reportItems = [
    ...(user?.isSuperAdmin || hasNavigableInspectionAction(effectivePermissions.data?.permissions, 'REPORT_VIEW') ? [
      { key: '/reports/workbench', icon: <FileTextOutlined />, label: <Link to="/reports/workbench">通报工作台</Link> },
    ] : []),
    ...(user?.isSuperAdmin || hasNavigableInspectionAction(effectivePermissions.data?.permissions, 'ARCHIVE_VIEW') ? [
      { key: '/reports/archive', icon: <FileTextOutlined />, label: <Link to="/reports/archive">通报归档</Link> },
    ] : []),
  ]
  const navItems = [workbenchItem, ...basicItems, ...configurationItems, ...visibleInspectionItems, ...reportItems]
  const menuItems: NonNullable<MenuProps['items']> = [workbenchItem]
  if (basicItems.length === 1) menuItems.push(basicItems[0])
  if (basicItems.length > 1) menuItems.push({ key: 'basic', icon: <AppstoreOutlined />, label: '基础功能', children: basicItems })
  if (configurationItems.length === 1) menuItems.push(configurationItems[0])
  if (configurationItems.length > 1) menuItems.push({ key: 'configuration', icon: <SettingOutlined />, label: '系统配置', children: configurationItems })
  if (visibleInspectionItems.length === 1) menuItems.push(visibleInspectionItems[0])
  if (visibleInspectionItems.length > 1) menuItems.push({ key: 'inspections', icon: <CheckSquareOutlined />, label: '日常检查', children: visibleInspectionItems })
  if (reportItems.length) menuItems.push({ key: 'reports', icon: <FileTextOutlined />, label: '通报管理', children: reportItems })
  const selectedKey = location.pathname === '/'
    ? '/'
    : navItems.find(item => item.key !== '/' && location.pathname.startsWith(item.key))?.key
  const selectedParent = basicItems.length > 1 && basicItems.some(item => item.key === selectedKey) ? 'basic'
    : configurationItems.length > 1 && configurationItems.some(item => item.key === selectedKey) ? 'configuration'
      : visibleInspectionItems.length > 1 && visibleInspectionItems.some(item => item.key === selectedKey) ? 'inspections'
        : reportItems.some(item => item.key === selectedKey) ? 'reports' : undefined
  const menuSignature = `${selectedKey ?? ''}:${selectedParent ?? ''}`
  const openKeys = openMenuState.signature === menuSignature ? openMenuState.keys : selectedParent ? [selectedParent] : []
  const accountItems = [
    ...(user?.isSuperAdmin || user?.systemRole === 'ADMIN' ? [
      { key: 'profile', icon: <UserOutlined />, label: '个人信息' },
      { key: 'security', icon: <LockOutlined />, label: '账户安全' },
      { type: 'divider' as const },
    ] : []),
    { key: 'logout', icon: <LogoutOutlined />, label: '退出登录', danger: true, disabled: signingOut },
  ]

  const onAccountAction = async ({ key }: { key: string }) => {
    if (key === 'logout') {
      if (signingOut) return
      setSigningOut(true)
      setLogoutFailed(false)
      try {
        await signOut()
        navigate('/login', { replace: true })
      } catch {
        setLogoutFailed(true)
      } finally {
        setSigningOut(false)
      }
      return
    }
    navigate(`/admin/${key}`)
  }

  return <Layout className="admin-shell">
    <a className="skip-link" href="#main-content">跳到主要内容</a>
    <Layout.Sider
      width={256}
      id="admin-navigation"
      aria-label="主导航"
      className="admin-sider"
      breakpoint="lg"
      collapsed={collapsed}
      collapsedWidth={80}
      onBreakpoint={(broken) => {
        if (broken) setCollapsed(true)
      }}
    >
      <div className="admin-brand">
        <img className="admin-brand-image" src="/logo.png" alt="日常管理系统图标" />
        <div className="admin-brand-text" aria-hidden={collapsed}>
          <strong>学生日常管理系统</strong>
          <span>湖南科技大学 · 计算机科学与工程学院</span>
        </div>
      </div>
      <Menu mode="inline" theme="light" inlineIndent={14} selectedKeys={selectedKey ? [selectedKey] : []} openKeys={openKeys} onOpenChange={keys => setOpenMenuState({ signature: menuSignature, keys: keys.filter(key => !openKeys.includes(key)).slice(-1) })} onClick={({ key }) => { if (key === '/') setOpenMenuState({ signature: menuSignature, keys: [] }) }} items={menuItems} />
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
        <WorkContextBar canManage={user?.isSuperAdmin === true} />
        <Dropdown menu={{ items: accountItems, onClick: onAccountAction }} placement="bottomRight"><Button type="text" className="account-trigger" aria-label={`${user?.name ?? '当前用户'}账户菜单`}><Space><Avatar className="account-mascot-avatar" size={34} src="/assets/characters/account-avatar.png" alt="" icon={<UserOutlined />} /><span className="account-copy"><strong>{user?.name}</strong><small>{accountRoleLabel}</small></span><Tag color="blue">在线</Tag><DownOutlined /></Space></Button></Dropdown>
      </Layout.Header>
      <Layout.Content id="main-content" className="admin-content">{logoutFailed && <Alert role="alert" type="error" showIcon title="暂时无法确认退出，请重试。" action={<Button loading={signingOut} disabled={signingOut} onClick={() => void onAccountAction({key:'logout'})}>重试退出</Button>} />}<Outlet /></Layout.Content>
    </Layout>
  </Layout>
}
