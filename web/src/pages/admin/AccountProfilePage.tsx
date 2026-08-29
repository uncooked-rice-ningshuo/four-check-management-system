import { IdcardOutlined, SafetyCertificateOutlined, UserOutlined } from '@ant-design/icons'
import { Avatar, Button, Card, Descriptions, Space, Tag, Typography } from 'antd'
import { Link } from 'react-router-dom'
import { useAuth } from '../../app/providers/authContext'

export function AccountProfilePage() {
  const { user } = useAuth()
  return <section className="admin-page account-page"><div className="page-heading"><div><Typography.Title level={2}>个人信息</Typography.Title><Typography.Paragraph>查看当前登录身份与账户状态。</Typography.Paragraph></div></div><Card><div className="profile-summary"><Avatar size={72} icon={<UserOutlined />} /><div><Typography.Title level={3}>{user?.name}</Typography.Title><Space><Tag color="blue">{user?.isSuperAdmin ? '超级管理员' : '普通用户'}</Tag><Tag color="green">账号正常</Tag></Space></div><Button type="primary"><Link to="/admin/security">账户安全</Link></Button></div><Descriptions bordered column={{ xs: 1, sm: 2 }} items={[
    { key: 'login', label: <Space><IdcardOutlined />登录账号</Space>, children: user?.loginName },
    { key: 'kind', label: '账号类型', children: user?.accountKind === 'SYSTEM' ? '系统账号' : '人员账号' },
    { key: 'student', label: '学号', children: user?.studentNo || '系统账号不使用学号' },
    { key: 'status', label: '账户状态', children: <Tag color="green">{user?.status}</Tag> },
    { key: 'permission', label: <Space><SafetyCertificateOutlined />权限级别</Space>, children: user?.isSuperAdmin ? '系统全部管理权限' : '按部门与角色授权' },
    { key: 'id', label: '账户标识', children: user?.id },
  ]} /></Card></section>
}
