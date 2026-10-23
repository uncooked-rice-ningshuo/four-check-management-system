import { Empty, Segmented, Space, Table, Tag, Typography } from 'antd'
import type { ImportRow, ImportRowValidationFilter } from '../../../entities/import-job/model/importJob'

import type { SupportedImportType } from '../../../shared/api/inspectionImportApi'

type PreviewFilter = ImportRowValidationFilter

type ImportPreviewTableProps = {
  typeCode?: SupportedImportType
  rows: ImportRow[]
  loading: boolean
  page: number
  pageSize: number
  total: number
  filter: PreviewFilter
  onFilterChange: (value: PreviewFilter) => void
  onPageChange: (page: number, pageSize: number) => void
}

export function ImportPreviewTable({ typeCode = 'ATTENDANCE_RATE', rows, loading, page, pageSize, total, filter, onFilterChange, onPageChange }: ImportPreviewTableProps) {
  return <Space orientation="vertical" size="middle" className="import-preview" style={{ width: '100%' }}>
    <div className="import-preview-toolbar">
      <Typography.Text strong>校验明细</Typography.Text>
      <Segmented<PreviewFilter>
        aria-label="筛选全部导入行的校验结果"
        value={filter}
        onChange={onFilterChange}
        options={[{ label: '全部', value: 'ALL' }, { label: '仅有效', value: 'VALID' }, { label: '仅错误', value: 'INVALID' }]}
      />
    </div>
    <Table<ImportRow>
      rowKey="id"
      loading={loading}
      dataSource={rows}
      locale={{ emptyText: <Empty description="没有符合条件的导入行" /> }}
      rowClassName={record => record.validationStatus === 'INVALID' ? 'import-preview-row-invalid' : ''}
      columns={[
        { title: 'Excel 行', dataIndex: 'rowNo', width: 88 },
        ...((typeCode === 'DORMITORY_HYGIENE' || typeCode === 'CLASSROOM_HYGIENE') ? [
          ...(typeCode === 'CLASSROOM_HYGIENE' ? [{title:'教室',render:(_:unknown,record:ImportRow)=>record.rawData.classroom_name||'—'},{title:'检查区域',render:(_:unknown,record:ImportRow)=>record.rawData.area_name||'—'}] : [{title:'寝室',render:(_:unknown,record:ImportRow)=>record.rawData.dormitory_name||'—'}]),
          {title:'检查日期',render:(_:unknown,record:ImportRow)=>record.rawData.inspection_date||'—'},
          {title:'时间/轮次',render:(_:unknown,record:ImportRow)=>record.rawData.inspection_time||record.rawData.inspection_round||'—'},
          {title:'检查结果',render:(_:unknown,record:ImportRow)=>record.rawData.result||'—'},
          {title:'责任班级',render:(_:unknown,record:ImportRow)=>record.rawData.responsible_class_names||'—'},
          {title:'责任学生学号',render:(_:unknown,record:ImportRow)=>record.rawData.responsible_student_nos||'—'},
          {title:'原因',render:(_:unknown,record:ImportRow)=>record.rawData.reason_name||'—'},
          {title:'情况说明',render:(_:unknown,record:ImportRow)=>record.rawData.description||'—'},
          {title:'备注',render:(_:unknown,record:ImportRow)=>record.rawData.remark||'—'},
        ] : [
        { title: '学号', render: (_: unknown, record: ImportRow) => record.normalizedRecord?.studentNo || record.rawData.student_no || record.rawData.studentNo || '—' },
        { title: '姓名', render: (_: unknown, record: ImportRow) => record.normalizedRecord?.studentName || record.rawData.student_name || record.rawData.studentName || '—' },
        { title: '班级', render: (_: unknown, record: ImportRow) => record.normalizedRecord?.className || record.rawData.class_name || record.rawData.className || '—' },
        ...(typeCode === 'MORNING_ATTENDANCE' ? [
          { title: '检查日期', width: 170, render: (_: unknown, record: ImportRow) => record.rawData.inspection_date || '—' },
          { title: '缺勤次数', width: 100, render: () => 1 },
          { title: '备注', render: (_: unknown, record: ImportRow) => record.rawData.remark || '—' },
        ] : typeCode === 'LATE_RETURN' ? [
          { title: '晚归时间', width: 170, render: (_: unknown, record: ImportRow) => [record.rawData.inspection_date,record.rawData.inspection_time].filter(Boolean).join(' ') || '—' },
          { title: '宿舍', render: (_: unknown, record: ImportRow) => record.rawData.dormitory_name || '—' },
          { title: '备注', render: (_: unknown, record: ImportRow) => record.rawData.remark || '—' },
        ] : [
        { title: '课程', render: (_: unknown, record: ImportRow) => record.normalizedRecord?.courseName || record.rawData.course_name || record.rawData.courseName || '—' },
        { title: '检查日期时间', width: 170, render: (_: unknown, record: ImportRow) => [record.rawData.absence_date, record.rawData.absence_time].filter(Boolean).join(' ') || '—' },
        { title: '节次', width: 100, render: (_: unknown, record: ImportRow) => record.normalizedRecord?.period || record.rawData.period || '—' },
        ]),
        ]),
        {
          title: '校验结果', width: 320, render: (_: unknown, record: ImportRow) => record.validationStatus === 'VALID'
            ? <Tag color="success">有效</Tag>
            : <Space orientation="vertical" size={2}><Tag color="error">需修正</Tag>{record.errors.map(error => <div key={`${error.code}-${error.field}`}><Typography.Text type="danger">{error.message}</Typography.Text><br /><Typography.Text type="secondary">建议：{error.suggestion}</Typography.Text></div>)}</Space>,
        },
      ]}
      pagination={{ current: page, pageSize, total, showSizeChanger: true, onChange: onPageChange, showTotal: value => `共 ${value} 行` }}
      scroll={{ x: 1170 }}
    />
  </Space>
}

export type { PreviewFilter }
