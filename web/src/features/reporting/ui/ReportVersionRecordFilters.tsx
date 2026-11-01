import { useState } from 'react'
import { Button, Form, Select, Space } from 'antd'
import type { ReportVersionRecordFilterOptions, ReportVersionRecordsQuery } from '../../../entities/reporting/model/reporting'

export type ReportVersionRecordFiltersValue = Pick<ReportVersionRecordsQuery, 'studentNumber' | 'studentName' | 'className' | 'grade'>

function options(values?: string[]) {
  return (values ?? []).map(value => ({ value, label: value }))
}

export function ReportVersionRecordFilters({ onApply, filterOptions }: {
  onApply: (filters: ReportVersionRecordFiltersValue) => void
  filterOptions?: ReportVersionRecordFilterOptions
}) {
  const [draftFilters, setDraftFilters] = useState<ReportVersionRecordFiltersValue>({})
  const apply = (values: ReportVersionRecordFiltersValue) => onApply({
    studentNumber: values.studentNumber?.trim() || undefined,
    studentName: values.studentName?.trim() || undefined,
    className: values.className?.trim() || undefined,
    grade: values.grade?.trim() || undefined,
  })
  const reset = () => {
    setDraftFilters({})
    onApply({})
  }

  return <Form layout="inline" className="report-version-record-filters" aria-label="筛选本版记录" onFinish={() => apply(draftFilters)}>
    <Form.Item label="学号" htmlFor="report-version-filter-student-number"><Select id="report-version-filter-student-number" aria-label="学号" allowClear showSearch optionFilterProp="label" options={options(filterOptions?.studentNumbers)} placeholder="搜索学号"
      value={draftFilters.studentNumber} onChange={value => setDraftFilters(current => ({ ...current, studentNumber: value || undefined }))} /></Form.Item>
    <Form.Item label="姓名" htmlFor="report-version-filter-student-name"><Select id="report-version-filter-student-name" aria-label="姓名" allowClear showSearch optionFilterProp="label" options={options(filterOptions?.studentNames)} placeholder="搜索姓名"
      value={draftFilters.studentName} onChange={value => setDraftFilters(current => ({ ...current, studentName: value || undefined }))} /></Form.Item>
    <Form.Item label="班级" htmlFor="report-version-filter-class"><Select id="report-version-filter-class" aria-label="班级" allowClear showSearch optionFilterProp="label" options={options(filterOptions?.classNames)} placeholder="搜索班级"
      value={draftFilters.className} onChange={value => setDraftFilters(current => ({ ...current, className: value || undefined }))} /></Form.Item>
    <Form.Item label="年级" htmlFor="report-version-filter-grade"><Select id="report-version-filter-grade" aria-label="年级" allowClear showSearch optionFilterProp="label" options={options(filterOptions?.grades)} placeholder="搜索年级"
      value={draftFilters.grade} onChange={value => setDraftFilters(current => ({ ...current, grade: value || undefined }))} /></Form.Item>
    <Space className="report-version-record-filter-actions">
      <Button type="primary" htmlType="button" onClick={() => apply(draftFilters)}>筛选</Button>
      <Button htmlType="button" onClick={reset}>重置</Button>
    </Space>
  </Form>
}
