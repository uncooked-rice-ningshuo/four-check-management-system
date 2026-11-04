import { expect, it, vi } from 'vitest'
import type { ProColumns } from '@ant-design/pro-components'
import { createInspectionFilterColumns, createInspectionProvenanceColumns, createInspectionSortColumn } from './inspectionColumns'

type SelectOption = { value: string; label: string }

const base = {
  semesters: [{ id: 'semester-1', label: '测试-第一学期' }],
  weeks: [{ id: 'week-1', label: '第 9 周' }],
  departments: [{ id: 'department-1', label: '学习部' }],
  grades: [{ id: '2024级', label: '2024级' }, { id: '2025级', label: '2025级' }],
  classes: [
    { id: 'class-2401', label: '测试-计科2401班', grade: '2024级' },
    { id: 'class-2501', label: '计科2501班', grade: '2025级' },
  ],
  onSemesterChange: () => {},
  onGradeChange: () => {},
}

function column(columns: ProColumns<unknown>[], dataIndex: string) {
  const found = columns.find(item => item.dataIndex === dataIndex)
  if (!found) throw new Error(`missing column ${dataIndex}`)
  return found
}

function optionsOf(columns: ProColumns<unknown>[], dataIndex: string): SelectOption[] {
  return (column(columns, dataIndex).fieldProps as { options?: SelectOption[] } | undefined)?.options ?? []
}

it('选择年级后只列出该年级的班级', () => {
  const columns = createInspectionFilterColumns<unknown>({ ...base, selectedGrade: '2024级' })
  expect(optionsOf(columns, 'classId').map(item => item.value)).toEqual(['class-2401'])
})

it('未选择年级时列出全部班级', () => {
  const columns = createInspectionFilterColumns<unknown>(base)
  expect(optionsOf(columns, 'classId').map(item => item.value)).toEqual(['class-2401', 'class-2501'])
})

it('班级缺少年级元数据时不做过滤，避免筛出空列表', () => {
  const columns = createInspectionFilterColumns<unknown>({
    ...base,
    classes: [{ id: 'class-2401', label: '测试-计科2401班' }, { id: 'class-2501', label: '计科2501班' }],
    selectedGrade: '2024级',
  })
  expect(optionsOf(columns, 'classId').map(item => item.value)).toEqual(['class-2401', 'class-2501'])
})

it('切换年级时把新值交给页面处理', () => {
  const onGradeChange = vi.fn()
  const columns = createInspectionFilterColumns<unknown>({ ...base, onGradeChange })
  const onChange = (column(columns, 'grade').fieldProps as { onChange?: (value?: string) => void }).onChange
  onChange?.('2025级')
  expect(onGradeChange).toHaveBeenCalledWith('2025级')
})

it('排序列提供检查时间与学号两种排序方向，且不出现在表格里', () => {
  const sort = createInspectionSortColumn<unknown>()
  expect(sort.dataIndex).toBe('sort')
  expect(sort.hideInTable).toBe(true)
  expect(Object.keys(sort.valueEnum as Record<string, unknown>)).toEqual(['occurredAtDesc', 'occurredAtAsc', 'studentNoAsc', 'studentNoDesc'])
})

it('排序列不回显为空，默认展示检查时间从新到旧', () => {
  const fieldProps = createInspectionSortColumn<unknown>().fieldProps as { allowClear?: boolean; placeholder?: string }
  expect(fieldProps.allowClear).toBe(false)
  expect(fieldProps.placeholder).toBe('检查时间从新到旧')
})

it('把录入人和部门提交人作为不同来源展示', () => {
  const columns = createInspectionProvenanceColumns<{ source: string; status: string; createdByName?: string; submitterName?: string; submittedByName?: string }>()
  expect(columns.map(item => item.title)).toEqual(['数据来源', '数据状态', '录入人', '部门提交人', '提交时间'])
  const creator = columns.find(item => item.dataIndex === 'createdByName')
  const submitter = columns.find(item => item.dataIndex === 'submittedByName')
  const row = { source: 'EXCEL', status: 'CONFIRMED', createdByName: '批次导入员', submittedByName: '部门部长' }
  expect(creator?.render && Reflect.apply(creator.render, null, [undefined, row, 0, {}, {}])).toBe('批次导入员')
  expect(submitter?.renderText && Reflect.apply(submitter.renderText, null, ['部门部长', row, 0, {}])).toBe('部门部长')
})
