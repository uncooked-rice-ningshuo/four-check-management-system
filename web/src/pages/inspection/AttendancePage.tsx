import { PlusOutlined, UploadOutlined } from '@ant-design/icons'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
import { Alert, Button, Empty, Space, Tag, Typography } from 'antd'
import { useMemo, useState } from 'react'
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../../app/providers/authContext'
import type { AttendanceFilters, AttendanceRecord, InspectionContext } from '../../entities/inspection/model/inspection'
import { useAttendanceFilterOptions, useAttendancePermissions, useCurrentAcademicWeek } from '../../features/attendance/hooks/useAttendancePermissions'
import { useAttendanceRecords } from '../../features/attendance/hooks/useAttendanceRecords'
import { importErrorMessage } from '../../features/import-job/model/importErrorMessages'
import { AttendanceImportDrawer } from '../../features/import-job/ui/AttendanceImportDrawer'
import { CourseCreatePanel } from '../../features/course-inspection/ui/CourseCreatePanel'
import { AttendanceMutationDialogs } from '../../features/attendance/ui/AttendanceMutationDialogs'
import { canMutateAttendance } from '../../features/attendance/hooks/canMutateAttendance'
import { hasNavigableInspectionAction } from '../../features/attendance/hooks/hasNavigableInspectionAction'
import { DepartmentSubmissionActions } from '../../features/department-submission/ui/DepartmentSubmissionActions'
import { COLLEGE_DEPARTMENT_OPTION_ID } from '../../entities/inspection/model/inspectionScope'
import { ApiError } from '../../shared/api/httpClient'

type AttendanceSearchValues = {
	semesterId?: string
	weekId?: string
	departmentId?: string
	classId?: string
	grade?: string
	keyword?: string
	status?: AttendanceFilters['status']
	source?: AttendanceFilters['source']
	sort?: AttendanceFilters['sort']
}

const statusLabels: Record<NonNullable<AttendanceFilters['status']>, string> = {
	DRAFT: '草稿',
	PENDING_REVIEW: '待审核',
	RETURNED: '已退回',
	CONFIRMED: '已确认',
	LOCKED: '已锁定',
}

const sourceLabels: Record<NonNullable<AttendanceFilters['source']>, string> = {
	MANUAL: '手工录入',
	EXCEL: 'Excel 导入',
	REPORT_CORRECTION: '通报修正',
	MIGRATION: '历史迁移',
}

const dateTimeFormatter = new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' })

export function AttendancePage() {
	const { user } = useAuth()
	const location = useLocation()
	const navigate = useNavigate()
	const [search, setSearch] = useSearchParams()
	const importBatchId = (location.state as { importBatchId?: string } | null)?.importBatchId
	const [manualOpen, setManualOpen] = useState(false)
  const [editId, setEditId] = useState<string>()
  const [deleting, setDeleting] = useState<AttendanceRecord[]>([])
  const [selection, setSelection] = useState<AttendanceRecord[]>([])
	const [drawerOpen, setDrawerOpen] = useState(() => Boolean(importBatchId))
	const permissions = useAttendancePermissions()
	const currentWeek = useCurrentAcademicWeek()
	const filterOptions = useAttendanceFilterOptions()
	const options = filterOptions.data

	const requestedSemesterId = search.get('semesterId')
	const currentSemesterId = currentWeek.data?.week?.semesterId
	const semesterId = options ? availableId(options.semesters, requestedSemesterId, currentSemesterId) : ''
	const semesterWeeks = options?.weeks.filter(week => week.semesterId === semesterId) ?? []
	const requestedWeekId = search.get('weekId')
	const currentWeekId = currentWeek.data?.week?.semesterId === semesterId ? currentWeek.data.week.id : undefined
	const weekId = availableId(semesterWeeks, requestedWeekId, currentWeekId)
	const attendanceTypeId = options?.inspectionType.id ?? ''
	const departmentId = options ? availableId(options.departments, search.get('departmentId')) : ''
	const classId = search.get('classId') ?? ''
	const grade = search.get('grade') ?? ''
	const context = useMemo<InspectionContext>(() => ({
		semesterId,
		weekId,
		inspectionTypeId: attendanceTypeId,
		departmentId,
		...(classId ? { classId } : {}),
		...(grade ? { grade } : {}),
	}), [attendanceTypeId, classId, departmentId, grade, semesterId, weekId])
	const page = positiveInteger(search.get('page'), 1)
	const pageSize = positiveInteger(search.get('pageSize'), 20)
	const filters: AttendanceFilters = {
		...context,
		keyword: search.get('keyword') ?? '',
		status: optionalStatus(search.get('status')),
		source: optionalSource(search.get('source')),
		sort: optionalSort(search.get('sort')) ?? 'occurredAtDesc',
		page,
		pageSize,
	}
	const listReady = Boolean(context.semesterId && context.weekId && context.inspectionTypeId)
	const records = useAttendanceRecords(filters, listReady)
	const canMutate = (record: AttendanceRecord) => Boolean(user && canMutateAttendance(record, user, permissions.data?.permissions, options?.semesters.find(semester => semester.id === record.semesterId)?.academicYearId))
  const selected = selection.filter(item => records.data?.items.some(record => record.id === item.id) && canMutate(item))
  const selectedDrafts = selected.flatMap(item => item.status === 'DRAFT' && item.semesterId && item.weekId && item.inspectionTypeId
    ? [{
      id: item.id, rowVersion: item.rowVersion, status: item.status,
      semesterId: item.semesterId, weekId: item.weekId, departmentId: item.departmentId ?? '', inspectionTypeId: item.inspectionTypeId,
      // The cart rebuilds its rows from these display fields, so keep the name snapshots the list already holds.
      typeCode: 'ATTENDANCE_RATE', studentNumber: item.studentNo, studentName: item.studentName,
      className: item.className, grade: item.grade, source: item.source, importBatchId: item.importBatchId,
      details: { date: item.occurredAt.slice(0, 10) },
    }] : [])
  const canReview = Boolean(user?.isSuperAdmin || (attendanceTypeId && hasNavigableInspectionAction(permissions.data?.permissions, 'INSPECTION_REVIEW', attendanceTypeId)))
  const canImport = Boolean(user?.isSuperAdmin || (attendanceTypeId && (user?.systemRole !== 'MEMBER' || !!user.primaryDepartmentId) && hasNavigableInspectionAction(permissions.data?.permissions, 'INSPECTION_ENTER', attendanceTypeId)))
	const error = records.error instanceof ApiError ? records.error : undefined
	const responsibleDepartmentNames = (options?.responsibleDepartments ?? []).map(item => item.name).join('、')
	const departmentCount = options?.departments.length ?? 0
	const showDepartmentFilter = departmentCount > 1 || (departmentCount === 1 && !departmentId)

	const replaceSearch = (values: AttendanceSearchValues, nextPage = 1, nextPageSize = pageSize) => {
		setSelection([])
		const next = new URLSearchParams(search)
		const entries: Record<keyof AttendanceSearchValues, string | undefined> = {
			semesterId: values.semesterId,
			weekId: values.weekId,
			departmentId: values.departmentId,
			classId: values.classId,
			grade: values.grade,
			keyword: values.keyword?.trim(),
			status: values.status,
			source: values.source,
			sort: values.sort,
		}
		Object.entries(entries).forEach(([key, value]) => value ? next.set(key, value) : next.delete(key))
		next.set('page', String(nextPage))
		next.set('pageSize', String(nextPageSize))
		setSearch(next, { state: location.state })
	}
	const searchValues: AttendanceSearchValues = {
		semesterId,
		weekId,
		departmentId,
		classId: classId || undefined,
		grade: grade || undefined,
		keyword: filters.keyword,
		status: filters.status,
		source: filters.source,
		sort: filters.sort,
	}
	const columns = useMemo<ProColumns<AttendanceRecord>[]>(() => {
		const semesterName = (id: string) => options?.semesters.find(item => item.id === id)?.name ?? '未知学期'
		const result: ProColumns<AttendanceRecord>[] = [
			{ title: '学期', dataIndex: 'semesterId', hideInTable: true, order: 9, valueType: 'select', fieldProps: { options: options?.semesters.map(item => ({ value: item.id, label: item.name })) ?? [], placeholder: '请选择学期', allowClear: false, showSearch: true, optionFilterProp: 'label' } },
			{ title: '周次', dataIndex: 'weekId', hideInTable: true, order: 8, valueType: 'select', fieldProps: { options: options?.weeks.map(item => ({ value: item.id, label: `${semesterName(item.semesterId)} · 第 ${item.weekNo} 周` })) ?? [], placeholder: '请选择周次', allowClear: false, showSearch: true, optionFilterProp: 'label' } },
		]
		if (showDepartmentFilter) {
			result.push({ title: '部门', dataIndex: 'departmentId', hideInTable: true, order: 7, valueType: 'select', fieldProps: { options: options?.departments.map(item => ({ value: item.id, label: item.name })) ?? [], placeholder: '全部有权部门', allowClear: true, showSearch: true, optionFilterProp: 'label' } })
		}
		result.push(
			{ title: '班级', dataIndex: 'classId', hideInTable: true, order: 6, valueType: 'select', fieldProps: { options: options?.classes.map(item => ({ value: item.id, label: item.name })) ?? [], placeholder: '全部班级', allowClear: true, showSearch: true, optionFilterProp: 'label' } },
			{ title: '年级', dataIndex: 'grade', hideInTable: true, order: 5, valueType: 'select', fieldProps: { options: options?.grades.map(item => ({ value: item, label: gradeLabel(item) })) ?? [], placeholder: '全部年级', allowClear: true } },
			{ title: '数据状态', dataIndex: 'status', hideInTable: true, order: 4, valueType: 'select', valueEnum: Object.fromEntries(Object.entries(statusLabels).map(([value, text]) => [value, { text }])), fieldProps: { placeholder: '全部状态', allowClear: true } },
			{ title: '数据来源', dataIndex: 'source', hideInTable: true, order: 3, valueType: 'select', valueEnum: Object.fromEntries(Object.entries(sourceLabels).map(([value, text]) => [value, { text }])), fieldProps: { placeholder: '全部来源', allowClear: true } },
			{ title: '关键字', dataIndex: 'keyword', hideInTable: true, order: 2, fieldProps: { autoComplete: 'off', spellCheck: false, allowClear: true, placeholder: '输入学号或姓名…' } },
			{ title: '排序', dataIndex: 'sort', hideInTable: true, order: 1, valueType: 'select', valueEnum: { occurredAtDesc: { text: '检查时间从新到旧' }, occurredAtAsc: { text: '检查时间从旧到新' }, studentNoAsc: { text: '学号升序' }, studentNoDesc: { text: '学号降序' } }, fieldProps: { allowClear: false } },
			{ title: '学号', dataIndex: 'studentNo', width: 140, search: false, copyable: true },
			{ title: '姓名', dataIndex: 'studentName', width: 110, search: false },
			{ title: '班级', dataIndex: 'className', width: 150, search: false },
			{ title: '年级', dataIndex: 'grade', width: 90, search: false, renderText: value => value ? gradeLabel(String(value)) : '—' },
			{ title: '课程', dataIndex: 'courseName', width: 180, search: false, ellipsis: true },
			{ title: '检查时间', dataIndex: 'occurredAt', width: 180, search: false, renderText: value => dateTimeFormatter.format(new Date(value)) },
			{ title: '节次', dataIndex: 'period', width: 100, search: false },
			{ title: '实际旷课学时', dataIndex: 'absenceHours', width: 125, search: false },
			{ title: '违纪计入倍数', dataIndex: 'penaltyMultiplier', width: 125, search: false, render: (_, record) => record.penaltyMultiplier ? `${record.penaltyMultiplier} 倍` : '历史待核对' },
			{ title: '违纪计入学时', dataIndex: 'penaltyHours', width: 125, search: false, render: (_, record) => record.penaltyHours ?? '历史待核对' },
			{ title: '数据来源', dataIndex: 'source', width: 110, search: false, render: (_, record) => <Tag>{sourceLabels[record.source] ?? record.source}</Tag> },
			{ title: '数据状态', dataIndex: 'status', width: 110, search: false, render: (_, record) => <Tag color={record.status === 'CONFIRMED' ? 'green' : record.status === 'RETURNED' ? 'red' : 'blue'}>{statusLabels[record.status as keyof typeof statusLabels] ?? record.status}</Tag> },
		)
		return result
	}, [options, showDepartmentFilter])

	const updateImportBatchId = (batchId?: string) => {
		navigate(`${location.pathname}${location.search}`, { replace: true, state: { ...(location.state ?? {}), importBatchId: batchId } })
	}

	return <section className="attendance-page" aria-labelledby="attendance-title">
		<div className="attendance-page-heading">
			<div>
				<Typography.Title id="attendance-title" level={2}>到课率检查</Typography.Title>
				{responsibleDepartmentNames && <Typography.Text type="secondary">负责部门：{responsibleDepartmentNames}</Typography.Text>}
			</div>
		</div>
		{(permissions.isError || currentWeek.isError) && <Alert role="alert" type="error" showIcon title="无法加载权限或当前周次，请稍后重试" />}
		{filterOptions.isError && <Alert role="alert" type="error" showIcon title="无法加载到课率筛选项，请稍后重试" />}
		{listReady && records.isError && <Alert role="alert" type={error?.status === 403 ? 'warning' : 'error'} showIcon title={importErrorMessage(error?.code)} action={<Button onClick={() => records.refetch?.()}>重试</Button>} />}
		<ProTable<AttendanceRecord, AttendanceSearchValues>
			key={`${attendanceTypeId}-${departmentId}-${semesterId}-${weekId}`}
			className="attendance-records-table"
			cardBordered
			rowKey="id"
			columns={[...columns, { title: '操作', valueType: 'option', width: 240, fixed: 'right', render: (_, record) => canMutate(record) ? <Space>{record.importBatchId && <Button type="link" onClick={() => { updateImportBatchId(record.importBatchId); setDrawerOpen(true) }}>导入批次</Button>}<Button type="link" onClick={() => setEditId(record.id)}>编辑</Button>{record.status === 'DRAFT' && <Button type="link" danger onClick={() => setDeleting([record])}>删除</Button>}</Space> : null }]}
      rowSelection={{ selectedRowKeys: selectedDrafts.map(record => record.id), onChange: (_, rows) => setSelection(rows), getCheckboxProps: record => ({ disabled: !canMutate(record) || record.status !== 'DRAFT' }) }}
			loading={permissions.isLoading || currentWeek.isLoading || filterOptions.isLoading || records.isLoading}
			dataSource={listReady && !records.isError ? records.data?.items ?? [] : []}
			headerTitle={<Space size={10}><span>到课率记录</span><Tag color="blue">共 {records.data?.total ?? 0} 条</Tag></Space>}
			search={{ labelWidth: 'auto', defaultCollapsed: false }}
			form={{ initialValues: searchValues }}
			onSubmit={values => replaceSearch({ ...searchValues, ...values })}
			onReset={() => replaceSearch({ semesterId, weekId, sort: 'occurredAtDesc' })}
			options={{ reload: () => void records.refetch(), density: true, setting: true }}
			toolBarRender={() => [...(canImport ? [<Button key="manual" icon={<PlusOutlined aria-hidden />} disabled={!options} onClick={() => setManualOpen(true)}>新增记录</Button>, <Button key="import" type="primary" icon={<UploadOutlined aria-hidden />} disabled={!listReady} onClick={() => setDrawerOpen(true)} aria-label="批量导入到课率">批量导入</Button>] : []), <DepartmentSubmissionActions key="submissions" inspectionTypeId={attendanceTypeId} listScope={context} selected={selectedDrafts} canReview={canReview} scopeLabels={{ semesters: options?.semesters.map(item => ({ id: item.id, label: item.name })), weeks: options?.weeks.map(item => ({ id: item.id, label: `第 ${item.weekNo} 周` })), departments: options?.departments.map(item => ({ id: item.id, label: item.name })), inspectionTypeName: '到课率' }} onChanged={() => { setSelection([]); void records.refetch() }} onOpenRecord={(id) => setEditId(id)} />, ...(selectedDrafts.length ? [<Button key="delete" danger onClick={() => setDeleting(selected.filter(item => item.status === 'DRAFT'))}>批量删除（{selectedDrafts.length}）</Button>] : [])]}
			pagination={{ current: page, pageSize, total: records.data?.total ?? 0, showSizeChanger: true, showQuickJumper: true, showTotal: total => `共 ${total} 条`, onChange: (nextPage, nextPageSize) => replaceSearch(searchValues, nextPage, nextPageSize) }}
			scroll={{ x: 1320 }}
			locale={{ emptyText: <Empty description={listReady ? '当前范围暂无到课率记录' : '请选择学期和周次'} /> }}
		/>
		{(editId || deleting.length > 0) && <AttendanceMutationDialogs editId={editId} deleting={deleting} memberDepartmentId={user?.systemRole === 'MEMBER' ? user.primaryDepartmentId : undefined} onClose={() => { setEditId(undefined); setDeleting([]); setSelection([]) }} />}
		{manualOpen && options && <CourseCreatePanel inspectionTypeId={options.inspectionType.id} typeCode="ATTENDANCE_RATE" title="到课率" memberDepartmentId={user?.systemRole === 'MEMBER' ? user.primaryDepartmentId : undefined} onClose={() => setManualOpen(false)} />}
		{drawerOpen && options && <AttendanceImportDrawer
			open={drawerOpen}
			memberDepartmentId={user?.systemRole === 'MEMBER' ? user.primaryDepartmentId : undefined}
			context={context}
			scopeOptions={options}
			departments={[...(options?.responsibleDepartments ?? []).filter(department => options?.departments.some(allowed => allowed.id === department.id)), ...(user?.systemRole !== 'MEMBER' ? (options?.departments ?? []).filter(department => department.id === COLLEGE_DEPARTMENT_OPTION_ID) : [])]}
			batchId={importBatchId}
			onBatchIdChange={updateImportBatchId}
			onClose={() => setDrawerOpen(false)}
		/>}
	</section>
}

function positiveInteger(value: string | null, fallback: number) {
	const parsed = Number(value)
	return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback
}

function availableId<T extends { id: string }>(options: T[], ...preferredIds: (string | null | undefined)[]) {
	for (const id of preferredIds) {
		if (id && options.some(option => option.id === id)) return id
	}
	return ''
}

function gradeLabel(value: string) {
	const normalized = value.trim()
	return normalized.endsWith('级') ? normalized : `${normalized} 级`
}

function optionalStatus(value: string | null): AttendanceFilters['status'] {
	return value && value in statusLabels ? value as AttendanceFilters['status'] : undefined
}

function optionalSource(value: string | null): AttendanceFilters['source'] {
	return value && value in sourceLabels ? value as AttendanceFilters['source'] : undefined
}

function optionalSort(value: string | null): AttendanceFilters['sort'] {
	return value && ['occurredAtDesc', 'occurredAtAsc', 'studentNoAsc', 'studentNoDesc'].includes(value) ? value as AttendanceFilters['sort'] : undefined
}
