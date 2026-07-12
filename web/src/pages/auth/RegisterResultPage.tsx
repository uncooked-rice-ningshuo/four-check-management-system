import { Button, Result } from 'antd'
import { useNavigate } from 'react-router-dom'

export function RegisterResultPage() {
  const navigate = useNavigate()
  return <Result status="success" title="账号注册成功" subTitle="当前阶段账号已直接激活，可以使用学号和密码登录。" extra={<Button type="primary" onClick={() => navigate('/login')}>返回登录</Button>} />
}
