import { InboxOutlined, UploadOutlined } from '@ant-design/icons'
import { Alert, Button, Card, Drawer, Empty, Input, Space, Table, Tag, Typography } from 'antd'
import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import type { AttendanceFilters, AttendanceRecord, InspectionContext } from '../../entities/inspection/model/inspection'
import { useAuth } from '../../app/providers/authContext'
import { useAttendancePermissions, canInspectionAction, useCurrentAcademicWeek } from '../../features/attendance/hooks/useAttendancePermissions'
import { useAttendanceRecords } from '../../features/attendance/hooks/useAttendanceRecords'
import { importErrorMessage } from '../../features/import-job/model/importErrorMessages'
import { ApiError } from '../../shared/api/httpClient'

export function AttendancePage() {
  const { user } = useAuth()
  const [search, setSearch] = useSearchParams()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [drawerOpen, setDrawerOpen] = useState(false)
	const permissions = useAttendancePermissions()
	const currentWeek = useCurrentAcademicWeek()
	const viewScope = permissions.data?.permissions.find(permission => permission.action === 'INSPECTION_VIEW' && permission.effect === 'ALLOW')?.scope
	const context = useMemo<InspectionContext>(() => {
		const classId = search.get('classId') ?? viewScope?.classId ?? ''
		const grade = search.get('grade') ?? viewScope?.grade ?? ''
		return {
			semesterId: search.get('semesterId') ?? viewScope?.semesterId ?? currentWeek.data?.week?.semesterId ?? '',
			weekId: search.get('weekId') ?? viewScope?.weekId ?? currentWeek.data?.week?.id ?? '',
			inspectionTypeId: search.get('inspectionTypeId') ?? viewScope?.inspectionTypeId ?? '',
			departmentId: search.get('departmentId') ?? user?.primaryDepartmentId ?? viewScope?.departmentId ?? '',
			...(classId ? { classId } : {}), ...(grade ? { grade } : {}),
		}
	}, [currentWeek.data?.week, search, user?.primaryDepartmentId, viewScope])
  const filters: AttendanceFilters = { ...context, keyword: search.get('keyword') ?? '', page, pageSize }
  const contextReady = Object.values(context).every(Boolean)
  const records = useAttendanceRecords(filters, contextReady)
  const canImport = canInspectionAction(permissions.data?.permissions, 'INSPECTION_ENTER', context)

  const updateSearch = (key: string, value: string) => {
    const next = new URLSearchParams(search)
    next.set(key, value)
    setPage(1)
    setSearch(next)
  }
  const error = records.error instanceof ApiError ? records.error : undefined

  return <section className="attendance-page" aria-labelledby="attendance-title">
    <div className="attendance-page-heading">
      <div><Typography.Title id="attendance-title" level={2}>到课率检查</Typography.Title><Typography.Text type="secondary">按学期、周次和部门查看日常检查记录</Typography.Text></div>
      {canImport && <Button type="primary" icon={<UploadOutlined />} onClick={() => setDrawerOpen(true)} aria-label="批量导入到课率">批量导入</Button>}
    </div>
    <Card className="attendance-context" title="业务范围">
      <div className="attendance-context-grid">
				<label>检查类型<Input aria-label="检查类型" value={context.inspectionTypeId} onChange={event => updateSearch('inspectionTypeId', event.target.value)} placeholder="请选择检查类型" /></label>
        <label>学期<Input aria-label="学期" value={context.semesterId} onChange={event => updateSearch('semesterId', event.target.value)} placeholder="请选择当前学期" /></label>
        <label>周次<Input aria-label="周次" value={context.weekId} onChange={event => updateSearch('weekId', event.target.value)} placeholder="请选择当前周次" /></label>
        <label>部门<Input aria-label="部门" value={context.departmentId} onChange={event => updateSearch('departmentId', event.target.value)} placeholder="请选择部门" /></label>
				<label>班级范围<Input aria-label="班级范围" value={context.classId ?? ''} onChange={event => updateSearch('classId', event.target.value)} placeholder="可选班级" /></label>
				<label>年级范围<Input aria-label="年级范围" value={context.grade ?? ''} onChange={event => updateSearch('grade', event.target.value)} placeholder="可选年级" /></label>
        <label>检索<Input.Search aria-label="按学号或姓名检索" value={filters.keyword} onChange={event => updateSearch('keyword', event.target.value)} placeholder="学号或姓名" /></label>
      </div>
    </Card>
		{(permissions.isError || currentWeek.isError) && <Alert role="alert" type="error" showIcon title="无法加载权限或当前周次，请稍后重试" />}
    {!contextReady && <Alert type="info" showIcon title="请先补全学期、周次和部门范围" />}
    {contextReady && records.isLoading && <div role="status" className="attendance-state">正在加载到课率记录</div>}
    {contextReady && records.isError && <Alert role="alert" type={error?.status === 403 ? 'warning' : 'error'} showIcon title={importErrorMessage(error?.code)} action={<Button onClick={() => records.refetch?.()}>重试</Button>} />}
    {contextReady && !records.isLoading && !records.isError && records.data?.items.length === 0 && <Empty image={<InboxOutlined />} description="当前范围暂无到课率记录" />}
    {contextReady && !records.isLoading && !records.isError && Boolean(records.data?.items.length) && <Table<AttendanceRecord>
      rowKey="id"
      dataSource={records.data?.items}
      columns={[
        { title: '学号', dataIndex: 'studentNo' }, { title: '姓名', dataIndex: 'studentName' }, { title: '班级', dataIndex: 'className' },
        { title: '课程', dataIndex: 'courseName' }, { title: '时间', dataIndex: 'occurredAt', render: value => new Date(value).toLocaleString('zh-CN') },
        { title: '节次', dataIndex: 'period' }, { title: '学时', dataIndex: 'absenceHours' },
        { title: '来源', dataIndex: 'source', render: value => value === 'EXCEL' ? 'Excel 导入' : '手工录入' },
        { title: '状态', dataIndex: 'status', render: value => <Tag>{value === 'DRAFT' ? '草稿' : value}</Tag> },
      ]}
      pagination={{ current: records.data?.page, pageSize: records.data?.pageSize, total: records.data?.total, showSizeChanger: true, onChange: (next, size) => { setPage(next); setPageSize(size) }, showTotal: total => `共 ${total} 条` }}
      scroll={{ x: 980 }}
    />}
    <Drawer title="批量导入到课率" aria-label="批量导入到课率" open={drawerOpen} onClose={() => setDrawerOpen(false)} destroyOnHidden>
		<Space orientation="vertical"><Typography.Text>导入将在当前学期、周次和部门范围内进行。</Typography.Text><Alert type="info" showIcon title="模板下载、上传预览与确认流程将在导入工作流中完成。" /></Space>
    </Drawer>
  </section>
}
