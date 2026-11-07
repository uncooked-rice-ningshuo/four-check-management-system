import { useState } from 'react'
import type { UIEvent } from 'react'
import { Button, Form, Select, Space } from 'antd'
import type { ReportVersionRecordFilterOptions, ReportVersionRecordsQuery } from '../../../entities/reporting/model/reporting'
import { nextReportFilterOptionCount, reportFilterOptionCount, reportFilterOptionPage } from '../model/reportRecordFilters'

export type ReportVersionRecordFiltersValue = Pick<ReportVersionRecordsQuery, 'studentNumber' | 'studentName' | 'className' | 'grade'>
type ReportFilterField = keyof ReportVersionRecordFiltersValue
const initialOptionCounts: Record<ReportFilterField, number> = {
  studentNumber: 50,
  studentName: 50,
  className: 50,
  grade: 50,
}
const initialSearchQueries: Record<ReportFilterField, string> = {
  studentNumber: '',
  studentName: '',
  className: '',
  grade: '',
}

function options(values: string[], visibleCount: number, searchQuery: string) {
  return reportFilterOptionPage(values, searchQuery, visibleCount).map(value => ({ value, label: value }))
}

export function ReportVersionRecordFilters({ onApply, filterOptions, className, ariaLabel = '筛选本版记录' }: {
  onApply: (filters: ReportVersionRecordFiltersValue) => void
  filterOptions?: ReportVersionRecordFilterOptions
  className?: string
  ariaLabel?: string
}) {
  const [draftFilters, setDraftFilters] = useState<ReportVersionRecordFiltersValue>({})
  const [visibleOptionCounts, setVisibleOptionCounts] = useState(initialOptionCounts)
  const [searchQueries, setSearchQueries] = useState(initialSearchQueries)
  const valuesByField: Record<ReportFilterField, string[]> = {
    studentNumber: filterOptions?.studentNumbers ?? [],
    studentName: filterOptions?.studentNames ?? [],
    className: filterOptions?.classNames ?? [],
    grade: filterOptions?.grades ?? [],
  }
  const matchingOptionCount = (field: ReportFilterField) => reportFilterOptionCount(valuesByField[field], searchQueries[field])
  const fieldOptions = (field: ReportFilterField) => options(valuesByField[field], visibleOptionCounts[field], searchQueries[field])
  const loadNextOptions = (field: ReportFilterField) => (event: UIEvent<HTMLDivElement>) => {
    const { scrollTop, clientHeight, scrollHeight } = event.currentTarget
    if (scrollHeight - scrollTop - clientHeight > 24) return
    setVisibleOptionCounts(current => ({
      ...current,
      [field]: nextReportFilterOptionCount(current[field], matchingOptionCount(field)),
    }))
  }
  const updateSearchOptions = (field: ReportFilterField, search: string) => {
    setSearchQueries(current => ({ ...current, [field]: search }))
    setVisibleOptionCounts(current => ({ ...current, [field]: initialOptionCounts[field] }))
  }
  const apply = (values: ReportVersionRecordFiltersValue) => onApply({
    studentNumber: values.studentNumber?.trim() || undefined,
    studentName: values.studentName?.trim() || undefined,
    className: values.className?.trim() || undefined,
    grade: values.grade?.trim() || undefined,
  })
  const reset = () => {
    setDraftFilters({})
    setVisibleOptionCounts(initialOptionCounts)
    setSearchQueries(initialSearchQueries)
    onApply({})
  }

  return <Form layout="inline" className={['report-version-record-filters', className].filter(Boolean).join(' ')} aria-label={ariaLabel} onFinish={() => apply(draftFilters)}>
    <Form.Item label="学号" htmlFor="report-version-filter-student-number"><Select id="report-version-filter-student-number" aria-label="学号" allowClear showSearch optionFilterProp="label" searchValue={searchQueries.studentNumber} options={fieldOptions('studentNumber')} onSearch={value => updateSearchOptions('studentNumber', value)} onPopupScroll={loadNextOptions('studentNumber')} placeholder="搜索学号"
      value={draftFilters.studentNumber} onChange={value => setDraftFilters(current => ({ ...current, studentNumber: value || undefined }))} /></Form.Item>
    <Form.Item label="姓名" htmlFor="report-version-filter-student-name"><Select id="report-version-filter-student-name" aria-label="姓名" allowClear showSearch optionFilterProp="label" searchValue={searchQueries.studentName} options={fieldOptions('studentName')} onSearch={value => updateSearchOptions('studentName', value)} onPopupScroll={loadNextOptions('studentName')} placeholder="搜索姓名"
      value={draftFilters.studentName} onChange={value => setDraftFilters(current => ({ ...current, studentName: value || undefined }))} /></Form.Item>
    <Form.Item label="班级" htmlFor="report-version-filter-class"><Select id="report-version-filter-class" aria-label="班级" allowClear showSearch optionFilterProp="label" searchValue={searchQueries.className} options={fieldOptions('className')} onSearch={value => updateSearchOptions('className', value)} onPopupScroll={loadNextOptions('className')} placeholder="搜索班级"
      value={draftFilters.className} onChange={value => setDraftFilters(current => ({ ...current, className: value || undefined }))} /></Form.Item>
    <Form.Item label="年级" htmlFor="report-version-filter-grade"><Select id="report-version-filter-grade" aria-label="年级" allowClear showSearch optionFilterProp="label" searchValue={searchQueries.grade} options={fieldOptions('grade')} onSearch={value => updateSearchOptions('grade', value)} onPopupScroll={loadNextOptions('grade')} placeholder="搜索年级"
      value={draftFilters.grade} onChange={value => setDraftFilters(current => ({ ...current, grade: value || undefined }))} /></Form.Item>
    <Space className="report-version-record-filter-actions">
      <Button type="primary" htmlType="button" autoInsertSpace={false} onClick={() => apply(draftFilters)}>筛选</Button>
      <Button htmlType="button" autoInsertSpace={false} onClick={reset}>重置</Button>
    </Space>
  </Form>
}
