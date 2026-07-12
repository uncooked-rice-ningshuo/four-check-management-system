import { Button, Card, Typography } from 'antd'
import { useAuth } from '../../app/providers/authContext'

export function HomePage() {
  const { user, signOut } = useAuth()
  return <main className="home"><Card><Typography.Title level={2}>工程基础环境已就绪</Typography.Title><p>当前登录账号：{user?.name}（{user?.studentNo}）</p><p>注册审核、部门、角色和业务权限将在规则确认后开发。</p><Button onClick={signOut}>退出登录</Button></Card></main>
}
