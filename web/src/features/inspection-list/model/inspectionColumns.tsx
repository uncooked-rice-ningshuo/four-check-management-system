import type { ProColumns } from '@ant-design/pro-components'
import { Tag } from 'antd'

export type InspectionFilterOption = { id: string; label: string; grade?: string }

export type InspectionFilterColumnsInput = {
  semesters: InspectionFilterOption[]
  weeks: InspectionFilterOption[]
  departments: InspectionFilterOption[]
  grades: InspectionFilterOption[]
  classes: InspectionFilterOption[]
  selectedSemesterId?: string
  onSemesterChange: (value?: string) => void
  onWeekChange?: () => void
  selectedGrade?: string
  onGradeChange: (value?: string) => void
}

const statusLabels = {
  DRAFT: '草稿',
  PENDING_REVIEW: '待审核',
  RETURNED: '已退回',
  CONFIRMED: '已确认',
  LOCKED: '已锁定',
}

const sourceLabels = {
  MANUAL: '手工录入',
  EXCEL: 'Excel 导入',
  REPORT_CORRECTION: '通报修正',
  MIGRATION: '历史迁移',
}

const statusColors: Record<string, string> = {
  DRAFT: 'default',
  PENDING_REVIEW: 'processing',
  RETURNED: 'error',
  CONFIRMED: 'success',
  LOCKED: 'blue',
}

/** Shared list filters keep every inspection type in the same order and use the same controls. */
export function createInspectionFilterColumns<T>({
  semesters,
  weeks,
  departments,
  grades,
  classes,
  selectedSemesterId,
  onSemesterChange,
  onWeekChange,
  selectedGrade,
  onGradeChange,
}: InspectionFilterColumnsInput): ProColumns<T>[] {
  // 选了年级就只列出该年级的班级；班级项没有年级元数据时保持全量，避免筛出空列表。
  const visibleClasses = selectedGrade && classes.some(item => item.grade)
    ? classes.filter(item => item.grade === selectedGrade)
    : classes
  return [
    { title: '学期', dataIndex: 'semesterId', hideInTable: true, order: 8, valueType: 'select', fieldProps: { options: semesters.map(item => ({ value: item.id, label: item.label })), placeholder: '请选择学期', allowClear: true, showSearch: true, optionFilterProp: 'label', onChange: (value: string | undefined) => onSemesterChange(value) } },
    { title: '周次', dataIndex: 'weekId', hideInTable: true, order: 7, valueType: 'select', fieldProps: { options: weeks.map(item => ({ value: item.id, label: item.label })), placeholder: '请选择周次', disabled: !selectedSemesterId, allowClear: true, showSearch: true, optionFilterProp: 'label', onChange: onWeekChange } },
    { title: '部门', dataIndex: 'departmentId', hideInTable: true, order: 6, valueType: 'select', fieldProps: { options: departments.map(item => ({ value: item.id, label: item.label })), placeholder: '全部有权部门', allowClear: true, showSearch: true, optionFilterProp: 'label' } },
    { title: '年级', dataIndex: 'grade', hideInTable: true, order: 5, valueType: 'select', fieldProps: { options: grades.map(item => ({ value: item.id, label: item.label })), placeholder: '全部年级', allowClear: true, showSearch: true, optionFilterProp: 'label', onChange: (value: string | undefined) => onGradeChange(value) } },
    { title: '班级', dataIndex: 'classId', hideInTable: true, order: 4, valueType: 'select', fieldProps: { options: visibleClasses.map(item => ({ value: item.id, label: item.label })), placeholder: '全部班级', allowClear: true, showSearch: true, optionFilterProp: 'label' } },
    { title: '关键字', dataIndex: 'keyword', hideInTable: true, order: 3, fieldProps: { autoComplete: 'off', spellCheck: false, allowClear: true, placeholder: '输入学号或姓名' } },
    { title: '数据状态', dataIndex: 'status', hideInTable: true, order: 2, valueType: 'select', valueEnum: Object.fromEntries(Object.entries(statusLabels).map(([value, text]) => [value, { text }])), fieldProps: { placeholder: '全部状态', allowClear: true } },
    { title: '数据来源', dataIndex: 'source', hideInTable: true, order: 1, valueType: 'select', valueEnum: Object.fromEntries(Object.entries(sourceLabels).map(([value, text]) => [value, { text }])), fieldProps: { placeholder: '全部来源', allowClear: true } },
  ]
}

type InspectionRecordProvenance = { source: string; status: string; submitterName?: string; createdByName?: string; submittedByName?: string; submittedAt?: string }

/** Shared sort control keeps the ordering choice identical on every inspection list. */
export function createInspectionSortColumn<T>(): ProColumns<T> {
  return {
    title: '排序',
    dataIndex: 'sort',
    hideInTable: true,
    order: 0,
    valueType: 'select',
    valueEnum: {
      occurredAtDesc: { text: '检查时间从新到旧' },
      occurredAtAsc: { text: '检查时间从旧到新' },
      studentNoAsc: { text: '学号升序' },
      studentNoDesc: { text: '学号降序' },
    },
    fieldProps: { placeholder: '检查时间从新到旧', allowClear: false },
  }
}

/** Keep original data entry separate from department submission attribution. */
export function createInspectionProvenanceColumns<T extends InspectionRecordProvenance>(): ProColumns<T>[] {
  return [
    { title: '数据来源', dataIndex: 'source', search: false, width: 120, render: (_, record) => <Tag>{sourceLabels[record.source as keyof typeof sourceLabels] ?? record.source}</Tag> },
    { title: '数据状态', dataIndex: 'status', search: false, width: 120, render: (_, record) => <Tag color={statusColors[record.status] ?? 'default'}>{statusLabels[record.status as keyof typeof statusLabels] ?? record.status}</Tag> },
    { title: '录入人', dataIndex: 'createdByName', search: false, width: 130, ellipsis: true, render: (_, record) => record.createdByName || record.submitterName || '账号不可用' },
    { title: '部门提交人', dataIndex: 'submittedByName', search: false, width: 130, ellipsis: true, renderText: value => value || '—' },
    { title: '提交时间', dataIndex: 'submittedAt', search: false, width: 170, renderText: value => typeof value === 'string' ? new Date(value).toLocaleString('zh-CN') : '—' },
  ]
}

