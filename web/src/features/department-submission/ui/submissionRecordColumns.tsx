import type { ColumnsType } from 'antd/es/table'
import type { SubmissionCartRecord } from '../../../entities/inspection/model/submissionCart'

const resultLabels: Record<string, string> = { EXCELLENT: '优秀', QUALIFIED: '合格', UNQUALIFIED: '不合格' }
const sourceLabels: Record<string, string> = { MANUAL: '手工录入', EXCEL: 'Excel 导入', REPORT_CORRECTION: '通报修正', MIGRATION: '历史迁移' }
const statusLabels: Record<string, string> = { DRAFT: '草稿', PENDING_REVIEW: '待审核', RETURNED: '已退回', CONFIRMED: '已确认', LOCKED: '已锁定' }
const detail = (record: SubmissionCartRecord, key: string) => {
  const value = record.details[key]
  return typeof value === 'string' && value.trim() ? value : typeof value === 'number' ? String(value) : '—'
}
const currentName = (label: string | undefined, id: unknown) => label || (typeof id === 'string' && id ? '名称不可用' : '—')
const people = (items: { label: string }[] | undefined) => items?.map(item => item.label).join('、') || '—'

/** Display the same business fields while selecting, reviewing and confirming a submission. */
export function submissionRecordColumns<T extends SubmissionCartRecord>(typeCode: string): ColumnsType<T> {
  const col = (title: string, key: string, width: number, render: (record: SubmissionCartRecord) => string): ColumnsType<T>[number] => ({ title, key, width, render: (_value, record) => render(record) })
  const hygiene = ['DORMITORY_HYGIENE', 'CLASSROOM_HYGIENE', 'PUBLIC_DORMITORY_HYGIENE'].includes(typeCode)
  const columns: ColumnsType<T> = hygiene ? [
    col('检查地点（当前名称）', 'location', 220, r => currentName(r.currentDormitoryName || r.currentAreaName, r.details.dormitoryId || r.details.areaId)),
    col('责任班级', 'classes', 180, r => people(r.responsibleClasses)),
    col('责任学生', 'students', 160, r => people(r.responsibleStudents)),
    ...(typeCode === 'PUBLIC_DORMITORY_HYGIENE' ? [col('责任寝室', 'rooms', 180, r => people(r.responsibleDormitories))] : []),
  ] : [
    col('学号', 'studentNumber', 130, r => r.studentNumber || '—'),
    col('姓名', 'studentName', 110, r => r.studentName || '—'),
    col('班级', 'className', 160, r => r.className || '—'),
    col('年级', 'grade', 90, r => r.grade || '—'),
  ]
  if (typeCode === 'ATTENDANCE_RATE' || typeCode === 'UNATTENDED_CLASSROOM') columns.push(col('课程（当前名称）', 'course', 180, r => currentName(r.currentCourseName, r.details.courseId)))
  if (['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'EVENING_SELF_STUDY'].includes(typeCode)) columns.push(col('教室（当前名称）', 'classroom', 180, r => currentName(r.currentClassroomName, r.details.classroomId)))
  if (typeCode === 'LATE_RETURN') columns.push(col('宿舍（当前名称）', 'dormitory', 200, r => currentName(r.currentDormitoryName, r.details.dormitoryId)))
  columns.push(col('检查日期', 'date', 125, r => detail(r, 'date')))
  if (typeCode !== 'MORNING_ATTENDANCE') columns.push(col('检查时间', 'time', 125, r => detail(r, 'time')))
  if (hygiene) columns.push(col('检查轮次', 'round', 120, r => detail(r, 'round')))
  if (['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'EVENING_SELF_STUDY'].includes(typeCode)) columns.push(
    col('节次', 'period', 100, r => detail(r, 'period')),
  )
  if (typeCode === 'ATTENDANCE_RATE' || typeCode === 'UNATTENDED_CLASSROOM') columns.push(col('旷课节次数', 'absenceHours', 110, r => detail(r, 'absenceHours')))
  if (typeCode === 'ATTENDANCE_RATE' || typeCode === 'EVENING_SELF_STUDY') columns.push(
    col('违纪计入倍数', 'penaltyMultiplier', 120, r => detail(r, 'penaltyMultiplier') === '—' ? '历史待核对' : `${detail(r, 'penaltyMultiplier')} 倍`),
    col('违纪计入次数', 'penaltyCount', 120, r => { const key = typeCode === 'EVENING_SELF_STUDY' ? 'penaltyCount' : 'penaltyHours'; return detail(r, key) === '—' ? '历史待核对' : detail(r, key) }),
  )
  if (typeCode === 'MORNING_ATTENDANCE') columns.push(col('缺勤次数', 'absenceCount', 100, r => detail(r, 'absenceCount')))
  if (hygiene) columns.push(
    col('检查结果', 'result', 100, r => resultLabels[detail(r, 'result')] ?? '结果不可用'),
    col('不合格原因', 'reason', 160, r => r.currentReasonLabel || (r.details.reasonCode ? '原因名称不可用' : '—')),
    col('情况说明', 'description', 180, r => detail(r, 'description')),
  )
  columns.push(col('备注', 'remark', 180, r => detail(r, 'remark')),
    col('来源', 'source', 160, r => r.source === 'EXCEL' && r.importRowNo ? `Excel 导入 · 第 ${r.importRowNo} 行` : sourceLabels[r.source] ?? '来源未知'),
    col('状态', 'status', 100, r => statusLabels[r.status] ?? '状态未知'))
  return columns
}
