import { Alert, Button, Drawer, Form, Select, Spin } from 'antd'
import { useState } from 'react'
import type { InspectionContext } from '../../../entities/inspection/model/inspection'
import { COLLEGE_DEPARTMENT_OPTION_ID, departmentOptionId } from '../../../entities/inspection/model/inspectionScope'
import { useCurrentAcademicWeek } from '../../attendance/hooks/useAttendancePermissions'
import { useImportContext } from '../hooks/useImportContext'
import { useImportBatch } from '../hooks/useImportBatch'
import { AttendanceImportDrawer } from './AttendanceImportDrawer'
import type { SupportedImportType } from '../../../shared/api/inspectionImportApi'

/** Import destination is selected independently of the surrounding list filters. */
export function InspectionImportScopePanel({ typeCode, inspectionTypeId, memberDepartmentId, batchId, onBatchIdChange, onClose }: {
  typeCode: Exclude<SupportedImportType, 'ATTENDANCE_RATE'>;
  inspectionTypeId: string; memberDepartmentId?: string; batchId?: string; onBatchIdChange: (id?: string) => void; onClose: () => void
}) {
  const label = typeCode === 'CLASSROOM_HYGIENE' ? '教室卫生' : typeCode === 'DORMITORY_HYGIENE' ? '宿舍卫生' : typeCode === 'LATE_RETURN' ? '晚归' : typeCode === 'MORNING_ATTENDANCE' ? '早锻炼/早签' : typeCode === 'EVENING_SELF_STUDY' ? '晚自习' : '无机课堂'
  const [selectedSemester, setSelectedSemester] = useState<string>()
  const [selectedWeek, setSelectedWeek] = useState<string>()
  const [chosen, setChosen] = useState<InspectionContext>()
  const current = useCurrentAcademicWeek()
  const batch = useImportBatch(batchId)
  const purpose = batchId ? 'view' : 'entry'
  const semesters = useImportContext({ purpose, kind: 'semesters' }, true, typeCode)
  const defaultWeek = current.data?.week
  const defaultSemester = semesters.data?.items.some(item => item.id === defaultWeek?.semesterId) ? defaultWeek?.semesterId : undefined
  const semesterId = batch.data?.semesterId ?? chosen?.semesterId ?? selectedSemester ?? defaultSemester
  const weeks = useImportContext({ purpose, kind: 'weeks', semesterId }, !!semesterId, typeCode)
  const weekId = selectedWeek ?? (defaultWeek && defaultWeek.semesterId === semesterId && weeks.data?.items.some(item => item.id === defaultWeek.id) ? defaultWeek.id : undefined)
  const destinationWeekId = batch.data?.weekId ?? chosen?.weekId ?? weekId
  const departments = useImportContext({ purpose, kind: 'departments', semesterId, weekId: destinationWeekId }, !!semesterId && !!destinationWeekId, typeCode)
  const queries = [current, semesters, weeks, departments, ...(batchId ? [batch] : [])]
  if (queries.some(query => query.isError)) return <Drawer open title={`${label}导入范围`} onClose={onClose}><Alert type="error" title="导入范围加载失败，请重试。" action={<Button onClick={() => queries.forEach(query => { if (query.isError) void query.refetch() })}>重试</Button>} /></Drawer>
  if (queries.some(query => query.isLoading)) return <Drawer open title={`${label}导入范围`} onClose={onClose}><Spin /></Drawer>
  if (batch.data && batch.data.inspectionTypeId !== inspectionTypeId) return <Drawer open title={`${label}导入范围`} onClose={onClose}><Alert type="error" title={`该批次不属于${label}检查。`} /></Drawer>
  const target = batch.data ? { semesterId: batch.data.semesterId, weekId: batch.data.weekId, inspectionTypeId: batch.data.inspectionTypeId, departmentId: departmentOptionId(batch.data.departmentId), ...(batch.data.classId ? { classId: batch.data.classId } : {}), ...(batch.data.grade ? { grade: batch.data.grade } : {}) } : chosen
  if (target) return <AttendanceImportDrawer open typeCode={typeCode} context={target} initialImportScope={target} memberDepartmentId={memberDepartmentId} batchId={batchId} onBatchIdChange={id => { if (!id && batch.data) setChosen(target); onBatchIdChange(id) }} onClose={onClose}
    scopeOptions={{ inspectionType: { id: inspectionTypeId, name: label }, semesters: (semesters.data?.items ?? []).map(item => ({ id: item.id, name: item.label })), weeks: (weeks.data?.items ?? []).map(item => ({ id: item.id, semesterId: semesterId ?? '', name: item.label })) }}
    departments={(departments.data?.items ?? []).map(item => ({ id: item.id, name: item.label }))} />
  return <Drawer open title={`${label}导入范围`} onClose={onClose}>
    <Form layout="vertical">
      <Form.Item label="导入学期"><Select aria-label="导入学期" value={semesterId} options={semesters.data?.items.map(item => ({ value: item.id, label: item.label }))} onChange={value => { setSelectedSemester(value); setSelectedWeek('') }} /></Form.Item>
      <Form.Item label="导入周次"><Select aria-label="导入周次" disabled={!semesterId} value={weekId} options={weeks.data?.items.map(item => ({ value: item.id, label: item.label }))} onChange={setSelectedWeek} /></Form.Item>
      <Button type="primary" disabled={!semesterId || !weekId} onClick={() => { if (semesterId && weekId) setChosen({ semesterId, weekId, inspectionTypeId, departmentId: memberDepartmentId ?? (departments.data?.items.some(item => item.id === COLLEGE_DEPARTMENT_OPTION_ID) ? COLLEGE_DEPARTMENT_OPTION_ID : '') }) }}>准备导入</Button>
    </Form>
  </Drawer>
}
