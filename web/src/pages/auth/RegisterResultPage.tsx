import { Button, Result } from 'antd'
import { useNavigate } from 'react-router-dom'

export function RegisterResultPage() {
  const navigate = useNavigate()
  return <Result status="success" title="注册申请已提交" subTitle="账号正在等待管理员审核，审核通过后可使用学号和密码登录。" extra={<Button type="primary" onClick={() => navigate('/login')}>返回登录</Button>} />
}
