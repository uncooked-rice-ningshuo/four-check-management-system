import { useState } from 'react'
import { ApartmentOutlined } from '@ant-design/icons'
import { Alert, Button, Drawer, Empty, Form, Select, Space, Table, Tabs, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { ReportIndicatorValues, ReportIndicators, ReportVersion } from '../../../entities/reporting/model/reporting'

type MetricRow = { key: string; grade: string; className: string; values: ReportIndicatorValues }
type CountMetricKey = 'eveningSelfStudy' | 'classroomHygiene' | 'dormitoryPersonalHygiene'
  | 'dormitoryPublicHygiene' | 'dormitoryHygieneTotal' | 'lateReturn'
type IndicatorCard = {
  key: string
  label: string
  value: string
  unit?: string
  details: string[]
  rate?: boolean
}
type IndicatorView = 'overview' | 'detail'
type MetricTablePagination = {
  current: number
  pageSize: number
  onChange: (page: number, pageSize: number) => void
}

const countFormatter = new Intl.NumberFormat('zh-CN')

function formatCount(value: number) {
  return countFormatter.format(value)
}

function displayRate(value: number | null | undefined) {
  return value === null || value === undefined ? '不可计算' : `${value.toFixed(2)}%`
}

function classMetricRows(indicators: ReportIndicators): MetricRow[] {
  return indicators.classes.map((item, index) => ({
    key: item.classId ?? `class-${index}-${item.grade}-${item.className}`,
    grade: item.grade,
    className: item.className,
    values: item,
  }))
}

function baseColumns(): ColumnsType<MetricRow> {
  return [
    { title: '年级', dataIndex: 'grade', key: 'grade', width: 100, render: (value: string) => value || '—' },
    { title: '班级', dataIndex: 'className', key: 'className', width: 180 },
  ]
}

function tableFor(rows: MetricRow[], columns: ColumnsType<MetricRow>, pagination: MetricTablePagination) {
  return rows.length > 0
    ? <Table<MetricRow>
      size="small"
      pagination={{
        ...pagination,
        total: rows.length,
        hideOnSinglePage: true,
        showSizeChanger: true,
        pageSizeOptions: [10, 20, 50, 100],
        showTotal: total => `共 ${total} 个班级`,
      }}
      rowKey="key"
      columns={[...baseColumns(), ...columns]}
      dataSource={rows}
      scroll={{ x: 'max-content', y: 440 }}
    />
    : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无班级指标明细" />
}

function countTab(rows: MetricRow[], key: CountMetricKey, title: string, pagination: MetricTablePagination, personTimeTitle?: string) {
  const columns: ColumnsType<MetricRow> = [
    { title: '原始记录数', key: 'raw', width: 120, render: (_value, row) => row.values[key]?.rawRecordCount ?? '—' },
    { title: '按周倍数计入次数', key: 'count', width: 160, render: (_value, row) => row.values[key]?.count ?? '—' },
  ]
  if (personTimeTitle) columns.push({
    title: personTimeTitle, key: 'personTime', width: 120,
    render: (_value, row) => row.values[key]?.personTimeCount ?? '—',
  })
  return {
    key,
    label: title,
    children: tableFor(rows, columns, pagination),
  }
}

function indicatorTabs(indicators: ReportIndicators, rows: MetricRow[], pagination: MetricTablePagination) {
  const tabs = []
  if (indicators.totals.attendanceRate) tabs.push({
    key: 'attendanceRate', label: '到课率', children: tableFor(rows, [
      { title: '应到人数', key: 'expected', width: 100, render: (_value, row) => row.values.attendanceRate?.expectedStudentCount ?? '—' },
      { title: '违纪人数', key: 'violation', width: 100, render: (_value, row) => row.values.attendanceRate?.violationStudentCount ?? '—' },
      { title: '实到人数', key: 'attended', width: 100, render: (_value, row) => row.values.attendanceRate?.attendedStudentCount ?? '—' },
      { title: '到课率', key: 'rate', width: 110, render: (_value, row) => displayRate(row.values.attendanceRate?.rate) },
    ], pagination),
  })
  if (indicators.totals.unattendedClassroom) tabs.push({
    key: 'unattendedClassroom', label: '无机课堂', children: tableFor(rows, [
      { title: '原始记录数', key: 'raw', width: 110, render: (_value, row) => row.values.unattendedClassroom?.rawRecordCount ?? '—' },
      { title: '违规次数', key: 'count', width: 100, render: (_value, row) => row.values.unattendedClassroom?.violationCount ?? '—' },
      { title: '违规人次', key: 'peopleTimes', width: 100, render: (_value, row) => row.values.unattendedClassroom?.violationPersonTimeCount ?? '—' },
      { title: '违规人数', key: 'people', width: 100, render: (_value, row) => row.values.unattendedClassroom?.violatingStudentCount ?? '—' },
    ], pagination),
  })
  if (indicators.totals.morningAttendance) tabs.push({
    key: 'morningAttendance', label: '早锻炼/早签', children: tableFor(rows, [
      { title: '计划天数', key: 'days', width: 100, render: (_value, row) => row.values.morningAttendance?.plannedDayCount ?? '—' },
      { title: '应到人次', key: 'expected', width: 100, render: (_value, row) => row.values.morningAttendance?.expectedPersonTimes ?? '—' },
      { title: '实际缺勤人次', key: 'absence', width: 120, render: (_value, row) => row.values.morningAttendance?.absencePersonTimes ?? '—' },
      { title: '出勤人次', key: 'attended', width: 100, render: (_value, row) => row.values.morningAttendance?.attendedPersonTimes ?? '—' },
      { title: '计入违纪次数', key: 'penalty', width: 120, render: (_value, row) => row.values.morningAttendance?.penaltyCount ?? '—' },
      { title: '出勤率', key: 'rate', width: 100, render: (_value, row) => displayRate(row.values.morningAttendance?.rate) },
    ], pagination),
  })
  if (indicators.totals.eveningSelfStudy) tabs.push(countTab(rows, 'eveningSelfStudy', '晚自习', pagination, '缺勤人次'))
  if (indicators.totals.classroomHygiene) tabs.push(countTab(rows, 'classroomHygiene', '教室卫生', pagination))
  if (indicators.totals.dormitoryPersonalHygiene) tabs.push(countTab(rows, 'dormitoryPersonalHygiene', '个人卫生', pagination))
  if (indicators.totals.dormitoryPublicHygiene) tabs.push(countTab(rows, 'dormitoryPublicHygiene', '宿舍公共卫生', pagination))
  if (indicators.totals.dormitoryHygieneTotal) tabs.push(countTab(rows, 'dormitoryHygieneTotal', '宿舍卫生合计', pagination))
  if (indicators.totals.lateReturn) tabs.push(countTab(rows, 'lateReturn', '晚归', pagination, '晚归人次'))
  return tabs
}

function indicatorCards(indicators: ReportIndicators): IndicatorCard[] {
  const cards: IndicatorCard[] = []
  const totals = indicators.totals
  const multiplier = indicators.inputs.weekMultiplier

  if (totals.attendanceRate) {
    const metric = totals.attendanceRate
    cards.push({
      key: 'attendanceRate', label: '到课率', value: displayRate(metric.rate), rate: true,
      details: [
        `实到 ${formatCount(metric.attendedStudentCount)} / ${formatCount(metric.expectedStudentCount)} 人`,
        `违纪 ${formatCount(metric.violationStudentCount)} 人`,
      ],
    })
  }
  if (totals.unattendedClassroom) {
    const metric = totals.unattendedClassroom
    cards.push({
      key: 'unattendedClassroom', label: '无机课堂违规', value: formatCount(metric.violationCount), unit: '次',
      details: [
        `原始记录 ${formatCount(metric.rawRecordCount)} 条 · 周倍数 ${multiplier}`,
        `违规人次 ${formatCount(metric.violationPersonTimeCount)} · 违规人数 ${formatCount(metric.violatingStudentCount)}`,
      ],
    })
  }
  if (totals.morningAttendance) {
    const metric = totals.morningAttendance
    cards.push({
      key: 'morningAttendance', label: '早锻炼/早签出勤率', value: displayRate(metric.rate), rate: true,
      details: [
        `出勤 ${formatCount(metric.attendedPersonTimes)} / ${formatCount(metric.expectedPersonTimes)} 人次`,
        `缺勤 ${formatCount(metric.absencePersonTimes)} 人次 · 计入 ${formatCount(metric.penaltyCount)} 次`,
      ],
    })
  }

  const countCards: { key: CountMetricKey; label: string; personTimeLabel?: string }[] = [
    { key: 'eveningSelfStudy', label: '晚自习缺勤', personTimeLabel: '缺勤人次' },
    { key: 'classroomHygiene', label: '教室卫生不合格' },
    { key: 'dormitoryPersonalHygiene', label: '个人卫生不合格' },
    { key: 'dormitoryPublicHygiene', label: '宿舍公共卫生不合格' },
    { key: 'dormitoryHygieneTotal', label: '宿舍卫生合计' },
    { key: 'lateReturn', label: '晚归', personTimeLabel: '晚归人次' },
  ]
  for (const item of countCards) {
    const metric = totals[item.key]
    if (!metric) continue
    cards.push({
      key: item.key, label: item.label, value: formatCount(metric.count), unit: '次',
      details: [
        `原始记录 ${formatCount(metric.rawRecordCount)} 条 · 周倍数 ${multiplier}`,
        ...(item.personTimeLabel && metric.personTimeCount !== undefined
          ? [`${item.personTimeLabel} ${formatCount(metric.personTimeCount)}`]
          : []),
      ],
    })
  }
  return cards
}

export function ReportVersionIndicators({ version, mode = 'detail' }: { version: ReportVersion; mode?: IndicatorView }) {
  const [breakdownOpen, setBreakdownOpen] = useState(false)
  const [gradeFilter, setGradeFilter] = useState<string>()
  const [classFilter, setClassFilter] = useState<string>()
  const [classPage, setClassPage] = useState(1)
  const [classPageSize, setClassPageSize] = useState(20)
  const content = version.content
  if (content.scopeLimited) return <Alert
    showIcon
    type="info"
    message={`V${version.versionNo} 为受限范围版本，不能按全周口径计算指标。`}
    description="本版本只包含账号有权查看和生成的通报数据；为避免把部分记录与全院班级人数混算，本版不保存全周指标。"
  />
  if (!content.indicators) return <Alert
    showIcon
    type="info"
    message={`V${version.versionNo} 没有指标快照`}
    description="历史版本不会使用当前班级人数、早签计划或周倍数补算；只有生成时保存的指标数据才属于该版本。"
  />
  if (content.statisticsRuleVersion === 'WEEKLY_INDICATORS_V2' && content.incompleteCells.length > 0) return <Alert
    showIcon
    type="warning"
    message="本版存在未完成来源，指标暂不可判定"
    description="请先核对未提交或未确认的检查来源；当前已纳入记录不足以证明零违纪或完整出勤。"
  />

  const { indicators } = content
  const cards = indicatorCards(indicators)
  const allClassRows = classMetricRows(indicators)
  const gradeOptions = [...new Set(allClassRows.map(row => row.grade).filter(Boolean))]
    .sort((left, right) => left.localeCompare(right, 'zh-CN'))
    .map(value => ({ value, label: value }))
  const classOptions = [...new Set(allClassRows
    .filter(row => !gradeFilter || row.grade === gradeFilter)
    .map(row => row.className))]
    .sort((left, right) => left.localeCompare(right, 'zh-CN'))
    .map(value => ({ value, label: value }))
  const visibleClassRows = allClassRows.filter(row =>
    (!gradeFilter || row.grade === gradeFilter) && (!classFilter || row.className === classFilter))
  const tabs = indicatorTabs(indicators, mode === 'overview' ? visibleClassRows : allClassRows, {
    current: classPage,
    pageSize: classPageSize,
    onChange: (page, pageSize) => { setClassPage(page); setClassPageSize(pageSize) },
  })
  const classBreakdown = <Tabs
    className="report-version-indicator-tabs"
    size="small"
    items={tabs}
    onChange={() => setClassPage(1)}
  />

  return <section aria-label={`V${version.versionNo} 周指标`} className="report-version-indicators">
    <header className="report-version-indicators-heading">
      <div>
        <Typography.Text strong>{mode === 'overview' ? '本版指标' : `V${version.versionNo} 周指标`}</Typography.Text>
        <Typography.Text type="secondary" className="report-version-indicators-meta">
          第 {version.versionNo} 版 · 本周按 {indicators.inputs.weekMultiplier} 倍计入 · 实际早签天数 {indicators.inputs.morningPlannedDates.length} 天
        </Typography.Text>
      </div>
      {mode === 'overview' && tabs.length > 0 && <Button
        className="report-version-indicator-breakdown-trigger"
        size="small"
        icon={<ApartmentOutlined aria-hidden="true" />}
        onClick={() => setBreakdownOpen(true)}
      >
        班级明细
      </Button>}
    </header>
    <div className="report-version-indicator-cards" aria-label="全周指标汇总">
      {cards.map(card => <article key={card.key} className={`report-version-indicator-card${card.rate ? ' report-version-indicator-card-rate' : ''}`}>
        <Typography.Text type="secondary" className="report-version-indicator-label">{card.label}</Typography.Text>
        <div className="report-version-indicator-value">
          <span>{card.value}</span>{card.unit && <Typography.Text type="secondary">{card.unit}</Typography.Text>}
        </div>
        <div className="report-version-indicator-details">
          {card.details.map(detail => <Typography.Text key={detail} type="secondary">{detail}</Typography.Text>)}
        </div>
      </article>)}
    </div>
    {tabs.length > 0 && mode === 'detail' && <div className="report-version-indicator-breakdown">{classBreakdown}</div>}
    {tabs.length === 0 && <Alert showIcon type="info" message="本版本未纳入可计算的指标类型。" />}
    {mode === 'overview' && <Drawer
      rootClassName="report-version-indicator-drawer"
      title={<div className="report-version-indicator-drawer-title"><Typography.Text strong>班级指标明细</Typography.Text><Typography.Text type="secondary">本版共 {allClassRows.length} 个班级</Typography.Text></div>}
      open={breakdownOpen}
      size={Math.min(1040, typeof window === 'undefined' ? 1040 : window.innerWidth)}
      onClose={() => { setBreakdownOpen(false); setGradeFilter(undefined); setClassFilter(undefined); setClassPage(1) }}
      destroyOnHidden
    >
      <Space orientation="vertical" size="middle" className="report-version-indicator-drawer-content">
        <Form layout="inline" className="report-version-indicator-filters" aria-label="筛选班级指标">
          <Form.Item label="年级">
            <Select
              aria-label="筛选年级"
              allowClear
              showSearch
              optionFilterProp="label"
              placeholder="全部年级"
              options={gradeOptions}
              value={gradeFilter}
              onChange={value => { setGradeFilter(value); setClassFilter(undefined); setClassPage(1) }}
            />
          </Form.Item>
          <Form.Item label="班级">
            <Select
              aria-label="筛选班级"
              allowClear
              showSearch
              optionFilterProp="label"
              placeholder="全部班级"
              options={classOptions}
              value={classFilter}
              onChange={value => { setClassFilter(value); setClassPage(1) }}
            />
          </Form.Item>
          <Button
            disabled={!gradeFilter && !classFilter}
            onClick={() => { setGradeFilter(undefined); setClassFilter(undefined); setClassPage(1) }}
          >重置</Button>
          <Typography.Text type="secondary" className="report-version-indicator-filter-count">
            显示 {visibleClassRows.length} / {allClassRows.length} 个班级 · 本版保存数据
          </Typography.Text>
        </Form>
        {classBreakdown}
      </Space>
    </Drawer>}
  </section>
}
