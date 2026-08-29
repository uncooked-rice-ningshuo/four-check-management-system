import { Button, Card, Typography } from 'antd'
import { useAuth } from '../../app/providers/authContext'
import { Link } from 'react-router-dom'

export function HomePage() {
  const { user, signOut } = useAuth()
  return <main className="home"><Card><Typography.Title level={2}>日常管理工作台</Typography.Title><p>当前登录账号：{user?.name}（{user?.loginName}）</p>{user?.isSuperAdmin && <p><Link to="/admin">进入超级管理员配置中心</Link></p>}<Button onClick={signOut}>退出登录</Button></Card></main>
}
