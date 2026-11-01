import { Card, Descriptions, Tabs } from 'antd'
import type { ReportingRecord } from '../../../entities/reporting/model/reporting'
import { reportRecordDisplaySections } from './ReportRecordTable'

type ReportRecordSnapshotCardProps = {
  record: ReportingRecord
  versionNo: number
  title?: string
}

export function ReportRecordSnapshotCard({ record, versionNo, title = `V${versionNo} · 当前版本记录` }: ReportRecordSnapshotCardProps) {
  const sections = reportRecordDisplaySections(record)

  return <Card className="report-record-snapshot-card report-correction-snapshot-card" size="small" title={title}>
    <Tabs className="report-record-snapshot-tabs" size="small" items={[...sections.map(section => ({
      key: section.key,
      label: section.title,
      children: <Descriptions size="small" column={{ xs: 1, sm: 2, md: 3 }} items={section.fields.map(item => ({
          key: item.key,
          label: item.label,
          children: item.value,
        }))} />,
    })), ...(record.sourceSubmissionId ? [{ key: 'source', label: '提交来源', children: <Descriptions size="small" column={{ xs: 1, sm: 2 }} items={[
      { key: 'department', label: '提交归属', children: `${record.sourceDepartmentName || '历史名称不可用'}（当前名称）` },
      { key: 'submitter', label: '提交人', children: `${record.submittedByName || '历史姓名不可用'}（当前姓名）` },
      { key: 'time', label: '提交时间', children: record.submittedAt ? new Date(record.submittedAt).toLocaleString('zh-CN') : '历史时间不可用' },
      { key: 'revision', label: '来源修订', children: `第 ${record.sourceRevisionNo ?? 0} 次提交` },
    ]} /> }] : [])]} />
  </Card>
}
