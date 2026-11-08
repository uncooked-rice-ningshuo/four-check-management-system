import { useState } from 'react'
import { Alert, Button, Empty, Modal, Space, Table, Tabs, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { ReportCountRow, ReportVersion, ReportingPeriod, ReportingRecord } from '../../../entities/reporting/model/reporting'
import { useReportVersionRecords } from '../model/queries'
import { reportSourceStatusLabel } from '../model/labels'
import { reportRecordColumns, reportRecordTableWidth, reportRecordTypeCodeForName, reportRecordUsesPersonFilters } from './ReportRecordTable'
import { ReportRecordSnapshotCard } from './ReportRecordSnapshotCard'
import { ReportVersionIndicators } from './ReportVersionIndicators'
import { ReportVersionRecordFilters, type ReportVersionRecordFiltersValue } from './ReportVersionRecordFilters'
import { groupVersionRows } from '../model/groupReportRows'

type ReportVersionSnapshotsProps = {
  period: Pick<ReportingPeriod, 'semesterId' | 'weekId'>
  version: ReportVersion
  onAccessDenied: () => void
  editable?: boolean
  onCorrect?: (record: ReportingRecord) => void
  onExclude?: (record: ReportingRecord) => void
  inlineRecordDetail?: boolean
  showIndicators?: boolean
  showHeading?: boolean
}

function rowKey(row: ReportCountRow) { return row.inspectionTypeId }

function ReportVersionCellRecords({
  period, versionNo, row, expanded, onAccessDenied, editable = false, onCorrect, onExclude, inlineRecordDetail = false,
}: {
  period: Pick<ReportingPeriod, 'semesterId' | 'weekId'>
  versionNo: number
  row: ReportCountRow
  expanded: boolean
  onAccessDenied: () => void
  editable?: boolean
  onCorrect?: (record: ReportingRecord) => void
  onExclude?: (record: ReportingRecord) => void
  inlineRecordDetail?: boolean
}) {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [filters, setFilters] = useState<ReportVersionRecordFiltersValue>({})
  const [selectedRecord, setSelectedRecord] = useState<ReportingRecord | null>(null)
  const query = useReportVersionRecords({
    semesterId: period.semesterId,
    weekId: period.weekId,
    versionNo,
    inspectionTypeId: row.inspectionTypeId,
    ...filters,
    page,
    pageSize,
  }, expanded && row.recordCount > 0, onAccessDenied)
  const recordTypeCode = query.data?.items[0]?.typeCode ?? reportRecordTypeCodeForName(row.inspectionType)
  const columns: ColumnsType<ReportingRecord> = [
    ...reportRecordColumns(recordTypeCode),
    { title: '提交归属', key: 'sourceUnit', width: 130, render: (_value: unknown, record: ReportingRecord) => record.sourceDepartmentName || '历史归属不可用' },
    { title: '记录明细', key: 'view', width: 96, fixed: 'right', render: (_value: unknown, record: ReportingRecord) =>
      <Button type="link" size="small" onClick={() => setSelectedRecord(record)}>查看</Button> },
    ...(editable ? [{ title: '操作', key: 'actions', width: 150, fixed: 'right' as const, render: (_value: unknown, record: ReportingRecord) => <Space size={4}>
      <Button type="link" size="small" onClick={() => onCorrect?.(record)}>修正</Button>
      <Button type="link" size="small" danger onClick={() => onExclude?.(record)}>排除</Button>
    </Space> }] : []),
  ]

  if (row.recordCount === 0) {
    return <div className="report-version-cell-empty">
      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={<Space orientation="vertical" size={2}>
        <Typography.Text strong>当前检查类型没有业务记录</Typography.Text>
        <Typography.Text type="secondary">{row.confirmedZeroReceiptCount > 0
          ? `已确认零记录回执 ${row.confirmedZeroReceiptCount} 份。`
          : `该数据单元状态：${reportSourceStatusLabel(row.dataStatus)}。`}</Typography.Text>
      </Space>} />
    </div>
  }

  const hasFilters = Object.values(filters).some(Boolean)
  const emptyText = query.isError
    ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="读取失败，请使用上方的重试操作。" />
    : hasFilters
      ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有符合筛选条件的本版记录。">
        <Button type="link" onClick={() => { setFilters({}); setPage(1) }}>清除筛选</Button>
      </Empty>
      : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前查看范围内没有可展示的本版记录。" />

  return <Space orientation="vertical" size="middle" style={{ display: 'flex' }}>
    {query.isError && <Alert type="error" showIcon title="此版本的记录明细加载失败" action={<Button onClick={() => void query.refetch()}>重试</Button>} />}
    {editable && <Typography.Text type="secondary">修正或排除会生成新版本；这里始终展示 V{versionNo} 保存时的快照。</Typography.Text>}
    {reportRecordUsesPersonFilters(recordTypeCode) && <ReportVersionRecordFilters filterOptions={query.data?.filterOptions}
      onApply={nextFilters => { setFilters(nextFilters); setPage(1) }} />}
    <Table<ReportingRecord>
      className="report-record-table"
      rowKey="id"
      size="middle"
      columns={columns}
      dataSource={query.data?.items ?? []}
      loading={query.isPending || query.isFetching}
      locale={{ emptyText }}
      scroll={{ x: reportRecordTableWidth(columns) }}
      pagination={{ current: page, pageSize, total: query.data?.total ?? 0, showSizeChanger: true,
        pageSizeOptions: [10, 20, 50], showTotal: total => `共 ${total} 条本版记录`,
        onChange: (nextPage, nextPageSize) => {
          if (nextPageSize !== pageSize) { setPageSize(nextPageSize); setPage(1) } else setPage(nextPage)
        } }}
    />
    {inlineRecordDetail && selectedRecord && <section className="report-archive-record-detail" aria-label="记录明细">
      <div className="report-archive-record-detail-heading"><Typography.Text strong>记录明细</Typography.Text><Button onClick={() => setSelectedRecord(null)}>收起</Button></div>
      <ReportRecordSnapshotCard record={selectedRecord} versionNo={versionNo} title={`V${versionNo} · 记录快照`} />
    </section>}
    {!inlineRecordDetail && <Modal className="report-record-detail-modal" title="记录明细" width={960} open={Boolean(selectedRecord)} footer={null}
      onCancel={() => setSelectedRecord(null)} destroyOnHidden>
      {selectedRecord && <ReportRecordSnapshotCard record={selectedRecord} versionNo={versionNo} title={`V${versionNo} · 记录快照`} />}
    </Modal>}
  </Space>
}

export function ReportVersionSnapshots({ period, version, onAccessDenied, editable = false, onCorrect, onExclude, inlineRecordDetail = false, showIndicators = true, showHeading = true }: ReportVersionSnapshotsProps) {
  const [activeCellKey, setActiveCellKey] = useState<string | undefined>()
  const contentRows = groupVersionRows(version.content.rows)
  if (contentRows.length === 0) return <>
    {showIndicators && <ReportVersionIndicators version={version} />}
    <div className="report-version-snapshots-empty">
      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`V${version.versionNo} 尚无可展示的数据单元。`} />
    </div>
  </>
  const currentActiveCellKey = activeCellKey ?? rowKey(contentRows[0])

  return <>
    {showIndicators && <ReportVersionIndicators version={version} />}
    <section className="report-version-snapshots" aria-label={`V${version.versionNo} 数据明细`}>
    {showHeading && <div className="report-version-snapshots-heading">
      <Typography.Text strong>V{version.versionNo} · {version.content.confirmedRecordCount} 条记录</Typography.Text>
      <Typography.Text type="secondary">列表字段按检查类型呈现；查看记录可核对版本中保存的完整明细。</Typography.Text>
    </div>}
    <Tabs className="report-version-snapshot-tabs" size="small"
      destroyOnHidden
      activeKey={currentActiveCellKey}
      onChange={setActiveCellKey}
      items={contentRows.map(row => {
        const key = rowKey(row)
        return {
          key,
          label: <span className="report-version-tab-label">
            <Typography.Text strong>{row.inspectionType}</Typography.Text>
            <span>{row.recordCount} 条{row.confirmedZeroReceiptCount > 0 ? ` · 零记录回执 ${row.confirmedZeroReceiptCount}` : ''}</span>
          </span>,
          children: <ReportVersionCellRecords
            key={`${version.id}:${key}`}
            period={period}
            versionNo={version.versionNo}
            row={row}
            expanded={currentActiveCellKey === key}
            onAccessDenied={onAccessDenied}
            editable={editable}
            onCorrect={onCorrect}
            onExclude={onExclude}
            inlineRecordDetail={inlineRecordDetail}
          />,
        }
      })}
    />
    </section>
  </>
}
