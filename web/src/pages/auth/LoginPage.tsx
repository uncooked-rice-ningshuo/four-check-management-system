import { LockOutlined, UserOutlined } from '@ant-design/icons'
import { App, Alert, Button, Form, Input, Typography } from 'antd'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useState } from 'react'
import type { User } from '../../entities/user/model/user'
import { useAuth } from '../../app/providers/authContext'
import { AuthLayout } from '../../shared/layouts/AuthLayout'

export function LoginPage() {
  const { message } = App.useApp()
  const { signIn, loading, recoveryError, recoverSession } = useAuth(); const navigate = useNavigate(); const location = useLocation()
  const [submitting, setSubmitting] = useState(false)
  const finishLogin = (user: User) => {
    const from = (location.state as { from?: string })?.from
    const canReturn = from && from !== '/' && from !== '/change-password'
    navigate(user.mustChangePassword ? '/change-password' : canReturn ? from : '/', { replace: true })
  }
  const retryRecovery = async () => {
    if (loading || submitting) return
    const recovered = await recoverSession()
    if (recovered) finishLogin(recovered)
  }
	const submit = async (values: { loginName: string; password: string }) => {
    if (loading || submitting) return
    setSubmitting(true)
    try { finishLogin(await signIn(values)) }
    catch (error) { message.error(error instanceof Error ? error.message : '登录失败，请稍后重试') }
    finally { setSubmitting(false) }
  }
  return <AuthLayout variant="login"><div className="auth-form"><Typography.Title level={1}>欢迎回来</Typography.Title><Typography.Paragraph className="subtitle">使用账号和密码登录系统</Typography.Paragraph>{recoveryError && <Alert role="alert" type="warning" showIcon title={recoveryError} action={<Button onClick={() => void retryRecovery()} loading={loading} disabled={loading || submitting}>重试恢复登录</Button>} />}<Form layout="vertical" size="large" onFinish={submit} requiredMark={false}><Form.Item label="账号" name="loginName" rules={[{ required: true, message: '请输入账号' }]}><Input prefix={<UserOutlined />} placeholder="普通用户填写学号，管理员填写 admin…" autoComplete="username" /></Form.Item><Form.Item label="密码" name="password" rules={[{ required: true, message: '请输入密码' }]}><Input.Password prefix={<LockOutlined />} placeholder="请输入密码…" autoComplete="current-password" /></Form.Item><Button type="primary" htmlType="submit" loading={submitting} disabled={loading || submitting} block className="primary-button">登录</Button></Form><p className="switch">还没有账号？ <Link to="/register">立即注册</Link></p><Alert type="info" showIcon title="普通账号注册后需等待管理员审核，审核通过后方可登录。" /></div></AuthLayout>
}
