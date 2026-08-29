import { Button, Card, Form, Input, Typography, message } from 'antd'
import { useNavigate } from 'react-router-dom'
import { changePassword } from '../../shared/api/authApi'
import { useAuth } from '../../app/providers/authContext'

export function ChangePasswordPage() {
  const navigate = useNavigate()
  const { signOut } = useAuth()
  const submit = async (values: { currentPassword: string; newPassword: string; confirmPassword: string }) => {
    try {
      await changePassword(values)
      await signOut()
      message.success('密码已更新，请重新登录')
      navigate('/login', { replace: true })
    } catch (error) {
      message.error((error as Error).message)
    }
  }
  return <main className="password-page"><Card><Typography.Title level={2}>首次登录，请修改密码</Typography.Title><Typography.Paragraph>新密码至少 12 位，修改后所有已有会话将失效。</Typography.Paragraph><Form layout="vertical" onFinish={submit}>
    <Form.Item name="currentPassword" label="当前密码" rules={[{ required: true }]}><Input.Password autoComplete="current-password" /></Form.Item>
    <Form.Item name="newPassword" label="新密码" rules={[{ required: true }, { min: 12, message: '新密码至少 12 位' }]}><Input.Password autoComplete="new-password" /></Form.Item>
    <Form.Item name="confirmPassword" label="确认新密码" dependencies={['newPassword']} rules={[{ required: true }, ({ getFieldValue }) => ({ validator: (_, value) => value === getFieldValue('newPassword') ? Promise.resolve() : Promise.reject(new Error('两次输入的密码不一致')) })]}><Input.Password autoComplete="new-password" /></Form.Item>
    <Button type="primary" htmlType="submit" block>更新密码</Button>
  </Form></Card></main>
}
