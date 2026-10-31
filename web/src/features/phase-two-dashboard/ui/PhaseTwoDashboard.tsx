import { CheckCircleOutlined, ClockCircleOutlined, EditOutlined, ExclamationCircleOutlined, TeamOutlined } from '@ant-design/icons'
import { Alert, Button, Card, Col, Empty, Progress, Row, Skeleton, Space, Statistic, Tag, Typography } from 'antd'
import { Link } from 'react-router-dom'
import type { DepartmentProgressStatus, PhaseTwoDashboardData } from '../api/dashboardClient'
import { usePhaseTwoDashboard } from '../model/usePhaseTwoDashboard'

const statusPresentation: Record<DepartmentProgressStatus, { label: string; color: string }> = {
	NOT_ENTERED: { label: '未录入', color: 'default' },
	ENTERING: { label: '录入中', color: 'processing' },
	PENDING_REVIEW: { label: '待审核', color: 'gold' },
	RETURNED: { label: '已退回', color: 'error' },
	COMPLETED: { label: '已完成', color: 'success' },
}

// Current workbench links only to the five inspection flows approved for this phase.
const activeInspectionRoutes: Record<string, string> = {
	ATTENDANCE_RATE: '/inspections/attendance',
	UNATTENDED_CLASSROOM: '/inspections/unattended-classroom',
	MORNING_ATTENDANCE: '/inspections/morning-attendance',
	DORMITORY_HYGIENE: '/inspections/dormitory-hygiene',
	LATE_RETURN: '/inspections/late-return',
}

function DepartmentStatusCard({ item }: { item: PhaseTwoDashboardData['departmentStatuses'][number] }) {
	const finished = item.confirmed + item.locked
	const percent = item.total === 0 ? 0 : Math.round((finished / item.total) * 100)
	const presentation = statusPresentation[item.status]
	const route = activeInspectionRoutes[item.inspectionTypeCode]
	return <Card className="department-progress-card">
		<div className="department-progress-heading">
			<div><Space size={8}><Typography.Text strong>{item.departmentName}</Typography.Text><Tag>{item.departmentId ? '部门整体' : '校级整体'}</Tag></Space><Typography.Title level={4}>{item.inspectionTypeName}</Typography.Title></div>
			<Tag color={presentation.color}>{presentation.label}</Tag>
		</div>
		<Progress percent={percent} status={item.status === 'RETURNED' ? 'exception' : undefined} />
		<div className="department-progress-counts">
			<span><strong>{item.total}</strong> 条记录</span>
			<span>草稿 {item.draft}</span>
			<span>待审核 {item.pendingReview}</span>
			<span>退回 {item.returned}</span>
			<span>已确认 {finished}</span>
		</div>
		<div className="department-progress-counts" aria-label="本周提交批次">
			<span><strong>{item.batchTotal}</strong> 个提交批次</span>
			<span>待审核 {item.batchPending}</span>
			<span>已退回 {item.batchReturned}</span>
			<span>已确认 {item.batchConfirmed}</span>
		</div>
		{route && <Button type="link" className="department-progress-action"><Link to={route}>查看{item.inspectionTypeName}</Link></Button>}
	</Card>
}

export function PhaseTwoDashboard({ userName }: { userName?: string }) {
	const dashboard = usePhaseTwoDashboard()
	if (dashboard.isLoading) return <section className="workbench-page"><Card><Skeleton active /></Card></section>
	if (dashboard.isError || !dashboard.data) return <section className="workbench-page"><Alert type="error" showIcon title="工作台数据加载失败" description="请稍后重试；如果问题持续存在，请联系系统管理员。" /></section>
	const { context, myTodos, departmentStatuses } = dashboard.data
	const contextLabel = context
		? [context.academicYearName, context.semesterName, context.weekNo ? `第 ${context.weekNo} 周` : undefined].filter(Boolean).join(' · ')
		: '当前日期未匹配到已启用的学期周次'

	return <section className="workbench-page">
		<div className="dashboard-hero workbench-hero">
			<div><Tag color="blue" icon={<CheckCircleOutlined />}>个人业务中控台</Tag><Typography.Title level={1}>工作台</Typography.Title><Typography.Paragraph>{userName ? `${userName}，` : ''}{contextLabel}</Typography.Paragraph></div>
			<div className="hero-seal">日常<br />管理</div>
		</div>
		{!context ? <Alert type="warning" showIcon title="当前日期未匹配到已启用的学期周次" description="工作台不会用其他周次或模拟数字替代，请联系管理员核对学期与周次配置。" /> : <>
			<Row gutter={[18, 18]} className="stat-grid workbench-stats">
				<Col xs={24} sm={12} xl={6}><Card><Statistic title="我的待办" value={myTodos.total} prefix={<ClockCircleOutlined />} /><Typography.Text type="secondary">仅统计由我创建且需要继续处理的记录</Typography.Text></Card></Col>
				<Col xs={24} sm={12} xl={6}><Card><Statistic title="我的草稿" value={myTodos.draft} prefix={<EditOutlined />} /><Typography.Text type="secondary">待补充或待提交</Typography.Text></Card></Col>
				<Col xs={24} sm={12} xl={6}><Card><Statistic title="退回给我" value={myTodos.returned} prefix={<ExclamationCircleOutlined />} /><Typography.Text type="secondary">需要根据意见修改</Typography.Text></Card></Col>
				<Col xs={24} sm={12} xl={6}><Card><Statistic title="可见业务" value={departmentStatuses.length} prefix={<TeamOutlined />} /><Typography.Text type="secondary">按有效权限展示部门或校级整体</Typography.Text></Card></Col>
			</Row>
			<div className="workbench-section-heading"><div><Typography.Title level={3}>检查业务状态</Typography.Title><Typography.Text type="secondary">以下数量是当前周、当前授权范围内的部门或校级实时统计。</Typography.Text></div></div>
			{departmentStatuses.length === 0
				? <Card><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前账号没有可展示的检查业务范围" /></Card>
				: <Row gutter={[18, 18]}>{departmentStatuses.map(item => <Col xs={24} xl={12} key={`${item.departmentId}:${item.inspectionTypeId}`}><DepartmentStatusCard item={item} /></Col>)}</Row>}
		</>}
	</section>
}
