import { LockOutlined, UserOutlined } from '@ant-design/icons'
import { Alert, Button, Form, Input, Typography, message } from 'antd'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../../app/providers/authContext'
import { AuthLayout } from '../../shared/layouts/AuthLayout'

export function LoginPage() {
  const { signIn } = useAuth(); const navigate = useNavigate(); const location = useLocation()
  const submit = async (values: { studentNo: string; password: string }) => { try { await signIn(values); navigate((location.state as { from?: string })?.from || '/') } catch (error) { message.error((error as Error).message) } }
  return <AuthLayout variant="login"><div className="auth-form"><Typography.Title level={1}>欢迎回来</Typography.Title><Typography.Paragraph className="subtitle">使用学号和密码登录系统</Typography.Paragraph><Form layout="vertical" size="large" onFinish={submit} requiredMark={false}><Form.Item label="学号" name="studentNo" rules={[{ required: true, message: '请输入学号' }]}><Input prefix={<UserOutlined />} placeholder="请输入学号" autoComplete="username" /></Form.Item><Form.Item label="密码" name="password" rules={[{ required: true, message: '请输入密码' }]}><Input.Password prefix={<LockOutlined />} placeholder="请输入密码" autoComplete="current-password" /></Form.Item><Button type="primary" htmlType="submit" block className="primary-button">登录</Button></Form><p className="switch">还没有账号？ <Link to="/register">立即注册</Link></p><Alert type="info" showIcon message="当前阶段注册成功后可直接登录；注册审核将在业务规则确认后接入。" /></div></AuthLayout>
}
