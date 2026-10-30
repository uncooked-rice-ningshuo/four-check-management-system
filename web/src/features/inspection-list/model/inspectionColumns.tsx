import type { ProColumns } from '@ant-design/pro-components'
import { Tag } from 'antd'

export type InspectionFilterOption = { id: string; label: string }

export type InspectionFilterColumnsInput = {
  semesters: InspectionFilterOption[]
  weeks: InspectionFilterOption[]
  departments: InspectionFilterOption[]
  grades: InspectionFilterOption[]
  classes: InspectionFilterOption[]
  selectedSemesterId?: string
  onSemesterChange: (value?: string) => void
  onWeekChange?: () => void
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
}: InspectionFilterColumnsInput): ProColumns<T>[] {
  return [
    { title: '学期', dataIndex: 'semesterId', hideInTable: true, order: 8, valueType: 'select', fieldProps: { options: semesters.map(item => ({ value: item.id, label: item.label })), placeholder: '请选择学期', allowClear: true, showSearch: true, optionFilterProp: 'label', onChange: (value: string | undefined) => onSemesterChange(value) } },
    { title: '周次', dataIndex: 'weekId', hideInTable: true, order: 7, valueType: 'select', fieldProps: { options: weeks.map(item => ({ value: item.id, label: item.label })), placeholder: '请选择周次', disabled: !selectedSemesterId, allowClear: true, showSearch: true, optionFilterProp: 'label', onChange: onWeekChange } },
    { title: '部门', dataIndex: 'departmentId', hideInTable: true, order: 6, valueType: 'select', fieldProps: { options: departments.map(item => ({ value: item.id, label: item.label })), placeholder: '全部有权部门', allowClear: true, showSearch: true, optionFilterProp: 'label' } },
    { title: '年级', dataIndex: 'grade', hideInTable: true, order: 5, valueType: 'select', fieldProps: { options: grades.map(item => ({ value: item.id, label: item.label })), placeholder: '全部年级', allowClear: true, showSearch: true, optionFilterProp: 'label' } },
    { title: '班级', dataIndex: 'classId', hideInTable: true, order: 4, valueType: 'select', fieldProps: { options: classes.map(item => ({ value: item.id, label: item.label })), placeholder: '全部班级', allowClear: true, showSearch: true, optionFilterProp: 'label' } },
    { title: '关键字', dataIndex: 'keyword', hideInTable: true, order: 3, fieldProps: { autoComplete: 'off', spellCheck: false, allowClear: true, placeholder: '输入学号或姓名' } },
    { title: '数据状态', dataIndex: 'status', hideInTable: true, order: 2, valueType: 'select', valueEnum: Object.fromEntries(Object.entries(statusLabels).map(([value, text]) => [value, { text }])), fieldProps: { placeholder: '全部状态', allowClear: true } },
    { title: '数据来源', dataIndex: 'source', hideInTable: true, order: 1, valueType: 'select', valueEnum: Object.fromEntries(Object.entries(sourceLabels).map(([value, text]) => [value, { text }])), fieldProps: { placeholder: '全部来源', allowClear: true } },
  ]
}

type InspectionRecordProvenance = { source: string; status: string; submitterName?: string }

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

/** Shared visible provenance fields make the submitting actor easy to find in every list. */
export function createInspectionProvenanceColumns<T extends InspectionRecordProvenance>(): ProColumns<T>[] {
  return [
    { title: '数据来源', dataIndex: 'source', search: false, width: 120, render: (_, record) => <Tag>{sourceLabels[record.source as keyof typeof sourceLabels] ?? record.source}</Tag> },
    { title: '数据状态', dataIndex: 'status', search: false, width: 120, render: (_, record) => <Tag color={statusColors[record.status] ?? 'default'}>{statusLabels[record.status as keyof typeof statusLabels] ?? record.status}</Tag> },
    { title: '提交人', dataIndex: 'submitterName', search: false, width: 130, ellipsis: true, renderText: value => value || '账号不可用' },
  ]
}

