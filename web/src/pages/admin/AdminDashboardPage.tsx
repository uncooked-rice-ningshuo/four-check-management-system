import { ApartmentOutlined, BookOutlined, CalendarOutlined, CheckCircleOutlined, DatabaseOutlined, IdcardOutlined, SafetyCertificateOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { Button, Card, Col, Row, Space, Statistic, Tag, Typography } from 'antd'
import { Link } from 'react-router-dom'
import { useAuth } from '../../app/providers/authContext'
import { getAcademicCalendar, getCatalogs, getPendingRegistrations, getUsers } from '../../shared/api/adminApi'

export function AdminDashboardPage({ embedded = false }: { embedded?: boolean }) {
	const { user } = useAuth()
	const isSuperAdmin = Boolean(user?.isSuperAdmin)
	const requests = useQuery({ queryKey: ['registration-requests'], queryFn: getPendingRegistrations, enabled: isSuperAdmin })
	const users = useQuery({ queryKey: ['admin-users'], queryFn: getUsers, enabled: isSuperAdmin })
	const catalogs = useQuery({ queryKey: ['admin-catalogs'], queryFn: getCatalogs, enabled: isSuperAdmin })
	const calendar = useQuery({ queryKey: ['academic-calendar'], queryFn: getAcademicCalendar, enabled: isSuperAdmin })
	const currentSemester = calendar.data?.semesters.find(item => item.enabled)
	if (!isSuperAdmin) {
		return <section className={`admin-page dashboard-page${embedded ? ' embedded-admin-dashboard' : ''}`}>
			{embedded ? <div className="workbench-section-heading"><div><Typography.Title level={3}>系统管理</Typography.Title><Typography.Text type="secondary">维护基础数据和字典选项。</Typography.Text></div></div> : <div className="dashboard-hero"><div><Tag color="blue" icon={<CheckCircleOutlined />}>系统管理员</Tag><Typography.Title level={1}>基础配置工作台</Typography.Title><Typography.Paragraph>维护学院基础数据和字典选项；用户、权限和组织配置由超级管理员负责。</Typography.Paragraph></div><div className="hero-seal">基础<br />配置</div></div>}
			<Row gutter={[18, 18]} className="stat-grid">
				<Col xs={24} md={12}><Card title="基础数据" extra={<DatabaseOutlined />}><Typography.Paragraph>维护学院、专业、班级、学生、宿舍、教室和课程等数据。</Typography.Paragraph><Button type="primary"><Link to="/admin/master-data">进入基础数据</Link></Button></Card></Col>
				<Col xs={24} md={12}><Card title="字典管理" extra={<BookOutlined />}><Typography.Paragraph>维护业务表单使用的受控选项、显示顺序和启用状态。</Typography.Paragraph><Button><Link to="/admin/dictionaries">进入字典管理</Link></Button></Card></Col>
			</Row>
		</section>
	}

  return <section className={`admin-page dashboard-page${embedded ? ' embedded-admin-dashboard' : ''}`}>
    {embedded ? <div className="workbench-section-heading"><div><Typography.Title level={3}>系统管理</Typography.Title><Typography.Text type="secondary">组织、人员、权限和时间基线配置。</Typography.Text></div></div> : <div className="dashboard-hero"><div><Tag color="blue" icon={<CheckCircleOutlined />}>系统运行正常</Tag><Typography.Title level={1}>工作台</Typography.Title><Typography.Paragraph>先把组织、人员与时间基线配置准确，再开始七类检查业务。</Typography.Paragraph></div><div className="hero-seal">七类<br />检查</div></div>}
    <Row gutter={[18, 18]} className="stat-grid">
      <Col xs={24} sm={12} xl={6}><Card><Statistic title="待审核注册" value={requests.data?.length ?? 0} prefix={<SafetyCertificateOutlined />} loading={requests.isLoading} /><Link to="/admin/registrations">前往处理</Link></Card></Col>
      <Col xs={24} sm={12} xl={6}><Card><Statistic title="系统用户" value={users.data?.length ?? 0} prefix={<IdcardOutlined />} loading={users.isLoading} /><Link to="/admin/users">管理用户</Link></Card></Col>
      <Col xs={24} sm={12} xl={6}><Card><Statistic title="启用部门" value={catalogs.data?.departments.filter(item => item.enabled).length ?? 0} prefix={<ApartmentOutlined />} loading={catalogs.isLoading} /><Link to="/admin/organization">配置组织</Link></Card></Col>
      <Col xs={24} sm={12} xl={6}><Card><Statistic title="已配置周次" value={calendar.data?.weeks.length ?? 0} prefix={<CalendarOutlined />} loading={calendar.isLoading} /><Link to="/admin/academic">配置校历</Link></Card></Col>
    </Row>
    <Row gutter={[18, 18]} className="dashboard-lower">
      <Col xs={24} lg={15}><Card title="开始使用" extra={<Tag color={currentSemester ? 'green' : 'gold'}>{currentSemester?.name ?? '尚未设置当前学期'}</Tag>}><div className="setup-list"><div><span>01</span><div><strong>审核并配置人员</strong><p>处理注册申请，为成员分配部门与组织角色。</p></div><Button><Link to="/admin/registrations">查看申请</Link></Button></div><div><span>02</span><div><strong>核对七类检查映射</strong><p>确认各部门的录入、查看、审核和汇总范围。</p></div><Button><Link to="/admin/organization">检查映射</Link></Button></div><div><span>03</span><div><strong>建立学期周次</strong><p>系统初始为空，由管理员按实际校历人工配置。</p></div><Button type="primary"><Link to="/admin/academic">配置校历</Link></Button></div></div></Card></Col>
      <Col xs={24} lg={9}><Card title="基础配置状态"><Space orientation="vertical" size={18} className="status-stack"><div><span>检查类型</span><strong>{catalogs.data?.inspectionTypes.length ?? 0} / 7</strong></div><div><span>组织角色</span><strong>{catalogs.data?.organizationRoles.length ?? 0} 项</strong></div><div><span>学年</span><strong>{calendar.data?.academicYears.length ?? 0} 个</strong></div><div><span>当前学期</span><strong>{currentSemester?.name ?? '未配置'}</strong></div></Space></Card></Col>
    </Row>
  </section>
}
