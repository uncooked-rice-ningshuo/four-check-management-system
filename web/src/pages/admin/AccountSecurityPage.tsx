import { LockOutlined, SafetyCertificateOutlined } from '@ant-design/icons'
import { App, Alert, Button, Card, Form, Input, Typography } from 'antd'
import { useNavigate } from 'react-router-dom'
import { useRef, useState } from 'react'
import { useAuth } from '../../app/providers/authContext'

type PasswordValues = { currentPassword: string; newPassword: string; confirmPassword: string }

export function AccountSecurityPage() {
  const { message } = App.useApp()
  const navigate = useNavigate()
  const { changePassword } = useAuth()
  const pending = useRef(false)
  const [submitting, setSubmitting] = useState(false)
  const submit = async (values: PasswordValues) => {
    if (pending.current) return
    pending.current = true
    setSubmitting(true)
    try {
      await changePassword(values)
      message.success('密码已更新，请使用新密码重新登录')
      navigate('/login', { replace: true })
    } catch (error) {
      message.error((error as Error).message)
    } finally {
      pending.current = false
      setSubmitting(false)
    }
  }
  return <section className="admin-page account-page"><div className="page-heading"><div><Typography.Title level={2}>账户安全</Typography.Title><Typography.Paragraph>修改密码后，当前账号的全部既有会话会立即失效。</Typography.Paragraph></div></div><Card className="security-card"><Alert type="info" showIcon icon={<SafetyCertificateOutlined />} title="密码至少 12 位，建议混合使用大小写字母、数字和符号。" /><Form layout="vertical" onFinish={submit} className="security-form">
    <Form.Item name="currentPassword" label="当前密码" rules={[{ required: true, message: '请输入当前密码' }]}><Input.Password prefix={<LockOutlined />} autoComplete="current-password" /></Form.Item>
    <Form.Item name="newPassword" label="新密码" rules={[{ required: true, message: '请输入新密码' }, { min: 12, message: '新密码至少 12 位' }]}><Input.Password prefix={<LockOutlined />} autoComplete="new-password" /></Form.Item>
    <Form.Item name="confirmPassword" label="确认新密码" dependencies={['newPassword']} rules={[{ required: true, message: '请再次输入新密码' }, ({ getFieldValue }) => ({ validator: (_, value) => value === getFieldValue('newPassword') ? Promise.resolve() : Promise.reject(new Error('两次输入的密码不一致')) })]}><Input.Password prefix={<LockOutlined />} autoComplete="new-password" /></Form.Item>
    <Button type="primary" htmlType="submit" loading={submitting} disabled={submitting}>更新密码并重新登录</Button>
  </Form></Card></section>
}
