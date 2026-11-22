import { useLayoutEffect, useRef, useState } from 'react'
import { Alert, Button, Drawer, Select, Typography, Space, Tag, Empty, type FormInstance } from 'antd'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
import { hasNavigableInspectionAction } from '../../features/attendance/hooks/hasNavigableInspectionAction'
import { useAttendancePermissions } from '../../features/attendance/hooks/useAttendancePermissions'
import { useWorkContext } from '../../features/work-context/useWorkContext'
import { useLedgerDetail, useLedgerExport, useSemesterLedger } from '../../features/discipline-ledger/useSemesterLedger'
import { emptyLedgerFilter, type LedgerStudent, type DetailRequest } from '../../shared/api/semesterLedgerApi'
import './SemesterLedgerPage.css'

const caseStates: Record<string, string> = { PENDING: '待处理', APPROVED: '已确定', DEFERRED: '已延迟', VOID: '已作废' }
const detailPagination = { defaultPageSize: 10, showSizeChanger: true, pageSizeOptions: [10, 25, 50] }

export function SemesterLedgerPage() {
  const [panel, setPanel] = useState<'sources' | 'cases'>('sources')
  const [detail, setDetail] = useState<LedgerStudent>()
  const workContext = useWorkContext()
  const [chosenSemester, setSemester] = useState<string>()
  const semester = chosenSemester ?? workContext.data?.week?.semesterId ?? ''
  const [filter, setFilter] = useState(emptyLedgerFilter)
  const [draft, setDraft] = useState(emptyLedgerFilter)
  const [page, setPage] = useState(1)
  const [size, setSize] = useState(50)
  const query = useSemesterLedger(semester, filter, page, size)
  const permissions = useAttendancePermissions()
  const canExport = hasNavigableInspectionAction(permissions.data?.permissions, 'DISCIPLINE_LEDGER_EXPORT')
  const download = useLedgerExport(semester, filter)
  const data = query.data
  const ledgerRef = useRef<HTMLElement>(null)
  useLayoutEffect(() => {
    const ledger = ledgerRef.current
    const viewport = ledger?.closest<HTMLElement>('.admin-content')
    const body = ledger?.querySelector<HTMLElement>('.ant-table-body')
    if (!ledger || !viewport || !body) return
    // Reserve the actual space occupied by the filters, headers, pagination and page padding.
    const fit = () => {
      const bounds = ledger.getBoundingClientRect()
      const available = viewport.getBoundingClientRect().bottom - bounds.top - viewport.scrollTop
        - parseFloat(getComputedStyle(viewport).paddingBottom)
      const chrome = bounds.height - body.getBoundingClientRect().height
      ledger.style.setProperty('--ledger-body-height', Math.max(64, Math.floor(available - chrome)) + 'px')
    }
    fit()
    const observer = new ResizeObserver(fit)
    observer.observe(viewport)
    observer.observe(ledger)
    return () => observer.disconnect()
  }, [query.isError, semester])
  const [sourceCode, setSourceCode] = useState('')
  const sourceColumn = data?.columns.find(c => c.code === sourceCode)
  const filterOptions = (key: 'grade' | 'major' | 'class') => {
    const rows = (data?.organizations ?? []).filter(o => (key === 'grade' || !draft.grade || o.grade === draft.grade)
      && (key !== 'class' || !draft.major || o.major === draft.major))
    if (key === 'class') return [...new Map(rows.filter(o => o.classId).map(o => [
      o.classId, { value: o.classId, label: o.class },
    ])).values()]
    return [...new Set(rows.map(o => o[key]).filter(Boolean))].map(value => ({ value, label: value }))
  }
  const identityColumns: ProColumns<LedgerStudent>[] = [
    { title: '姓名', key: 'name', dataIndex: 'name', fixed: 'left', width: 88, disable: true, render: (_, row) => <Button type="link" size="small" onClick={() => { setPanel('sources'); setSourceCode(''); setDetail(row) }}>{row.name}</Button> },
    { title: '学号', key: 'number', dataIndex: 'number', width: 116 },
    { title: '年级', key: 'grade', dataIndex: 'grade', width: 76 },
    { title: '班级', key: 'className', dataIndex: 'className', width: 128 },
  ]
  const metrics = (section: number): ProColumns<LedgerStudent>[] => (data?.columns ?? []).filter(c => c.section === section).map(c => ({
      title: c.name + '（' + c.unit + '）', key: c.code, width: 132, align: 'right' as const,
      render: (_: unknown, row: LedgerStudent) => {
        const amount = row.values[c.code]
        if (amount === null) return '待核对'
        if (!amount) return 0
        return <button className="ledger-amount" onClick={() => { setPanel('sources'); setSourceCode(c.code); setDetail(row) }}>{amount}</button>
      },
    }))
  const changeScope = (form: FormInstance, key: string, value?: string) => {
    if (key === 'grade') {
      form.setFieldsValue({ major: undefined, class: undefined })
      setDraft({ ...emptyLedgerFilter, grade: value ?? '' })
    } else if (key === 'major') {
      form.setFieldsValue({ class: undefined })
      setDraft(previous => ({ ...previous, major: value ?? '', class: '' }))
    }
  }
  const searchColumns: ProColumns<LedgerStudent>[] = (['grade', 'major', 'class'] as const).map((key, index) => ({
    title: ['年级', '专业', '班级'][index], dataIndex: key, key: 'filter-' + key, valueType: 'select', hideInTable: true, hideInSetting: true,
    fieldProps: form => ({ 'aria-label': ['年级', '专业', '班级'][index], allowClear: true, options: filterOptions(key), onChange: (value?: string) => changeScope(form, key, value) }),
  }))
  searchColumns.push({ title: '姓名或学号', dataIndex: 'search', hideInTable: true, hideInSetting: true, fieldProps: { allowClear: true } })
  const columns: ProColumns<LedgerStudent>[] = [
    ...searchColumns,
    { title: '基本信息', key: 'identity', search: false, children: identityColumns.map(c => ({ ...c, search: false })) },
    { title: '最新处分事项', key: 'latestCase', search: false, width: 235, disable: true, render: (_, row) =>
      row.cases?.length ? row.cases.map(item => {
        const group = data?.columns.find(column => column.groupKey && column.groupKey === item.groupKey)
        return <div key={item.id}><Button type="link" size="small" onClick={() => { setPanel('cases'); setSourceCode(group?.code ?? ''); setDetail(row) }}>
          {group?.name ?? item.groupName ?? '累计项'} · {item.tier} · {caseStates[item.status]}
        </Button></div>
      }) : row.cases ? '暂无处分' : '处分数据未提供' },
    ...['原始日常检查', '组合累计项'].flatMap((title, section) => {
      const children = metrics(section)
      return children.length ? [{ title, key: 'metrics-' + section, search: false as const, children: children.map(c => ({ ...c, search: false as const })) }] : []
    }),
    { title: '操作', key: 'actions', search: false, width: 160, fixed: 'right', disable: true, render: (_, row) =>
      <><Button type="link" size="small" onClick={() => { setPanel('sources'); setSourceCode(''); setDetail(row) }}>检查记录</Button>
      <Button type="link" size="small" onClick={() => { setPanel('cases'); setSourceCode(''); setDetail(row) }}>处分事项</Button></> },
  ]
  return <section ref={ledgerRef} className="semester-ledger attendance-page">
    <header className="attendance-page-heading"><Typography.Title level={2}>学期违纪台账</Typography.Title><Select aria-label="学年与学期" placeholder="选择学年与学期" value={semester || undefined}
      options={data?.semesters} onChange={id => { setSemester(id); setDraft(emptyLedgerFilter); setFilter(emptyLedgerFilter); setPage(1) }} /></header>
    {query.isError ? <Alert type="error" showIcon title="学期违纪台账加载失败" description={query.error.message} action={<Button onClick={() => void query.refetch()}>重试</Button>} /> :
      <><p>{semester ? '已定稿周次：' + (data?.weeks.join('、') || (query.isFetching ? '读取中' : '暂无')) : '选择学期查看定稿数据'}{semester && data && (data.ruleVersion ? ' · 处分规则 V' + data.ruleVersion : ' · 处分规则尚未发布')}</p>
      {!!data?.unassigned && <Alert type="warning" title={data.unassigned + ' 条定稿明细未关联个人学号，未计入个人累计'} />}
      {download.isError && <Alert type="error" title={download.error.message} />}
      <ProTable<LedgerStudent> className="attendance-records-table" cardBordered rowKey="number" size="small" bordered headerTitle={<Space size={10}>学生违纪台账{data && <Tag color="blue">共 {data.total} 人</Tag>}</Space>}
        locale={{emptyText:<Empty description={semester ? '当前范围暂无违纪记录' : '请选择学期'} />}} columns={columns} dataSource={data?.items} loading={query.isFetching}
        key={semester} search={{ labelWidth: 'auto', defaultCollapsed: false, collapseRender: false, span: { xs: 24, sm: 12, md: 8, lg: 6, xl: 6, xxl: 4 } }}
        onSubmit={values => { setFilter({ grade: values.grade ?? '', major: values.major ?? '', class: values.class ?? '', search: (values.search ?? '').trim() }); setPage(1) }}
        onReset={() => { setDraft(emptyLedgerFilter); setFilter(emptyLedgerFilter); setPage(1) }}
        options={{ setting: true, density: false, reload: false }}
        toolBarRender={() => canExport ? [<Button size="small" key="export" disabled={!semester} loading={download.isPending} onClick={() => download.mutate()}>导出 Excel</Button>] : []}
        scroll={{ x: 'max-content', y: 'var(--ledger-body-height)' }}
        pagination={{ current: page, pageSize: size, total: data?.total, showSizeChanger: true, pageSizeOptions: [25, 50, 100], onChange: (p, n) => { setPage(size === n ? p : 1); setSize(n) } }} />
      <Drawer title={detail?.name + ' · 检查记录'} open={!!detail && panel === 'sources'} onClose={() => setDetail(undefined)} size="large">
        {detail && panel === 'sources' && <>
        {!!sourceColumn?.members.length && <p>{sourceColumn.members.map(m => {
          const value = detail?.values[m.code]
          return m.operator + ' ' + m.name + ' ' + (value === null ? '待核对' : value ?? 0) + ' × ' + m.factor
        }).join(' ')} = {detail?.values[sourceColumn.code]} {sourceColumn.unit}</p>}
        <LedgerDetails key={detail.number + sourceCode} kind="sources" semesterId={semester} studentNumber={detail.number}
          groupKey={sourceColumn?.groupKey} code={!sourceColumn?.section ? sourceCode : undefined} weeks={data?.weeks} columns={data?.columns} />
        </>}
      </Drawer>
      <Drawer title={detail?.name + ' · 处分事项'} open={!!detail && panel === 'cases'} onClose={() => setDetail(undefined)} size="large">
        {detail && panel === 'cases' &&
          <LedgerDetails key={detail.number + sourceCode} kind="cases" semesterId={semester} studentNumber={detail.number}
            groupKey={sourceColumn?.groupKey} />
        }
      </Drawer>
      </>}
  </section>
}

function LedgerDetails(props: Pick<DetailRequest, 'kind' | 'semesterId' | 'studentNumber' | 'groupKey' | 'code'> & {
  weeks?: number[]; columns?: { name: string; code: string; section: number; unit: string }[]
}) {
  const { weeks, columns, ...scope } = props
  const [filters, setFilters] = useState<Partial<DetailRequest>>({})
  const [page, setPage] = useState(1)
  const [size, setSize] = useState(10)
  const query = useLedgerDetail({ ...scope, ...filters, page, pageSize: size })
  const pagination = { ...detailPagination, current: page, pageSize: size, total: query.data?.total,
    onChange: (next: number, n: number) => { setPage(n === size ? next : 1); setSize(n) } }
  const tableProps = {
    rowKey: 'id', size: 'small' as const, pagination, loading: query.isFetching,
    search: { labelWidth: 'auto' as const, defaultCollapsed: false }, options: false as const,
    onSubmit: (values: Partial<DetailRequest>) => { setPage(1); setFilters(values) },
    onReset: () => { setPage(1); setFilters({}) },
  }
  const error = query.isError && <Alert type="error" title="明细读取失败" action={<Button onClick={() => void query.refetch()}>重试</Button>} />
  return <>{error}{scope.kind === 'sources' ?
    <ProTable {...tableProps} dataSource={query.data?.sources} columns={[
      { title: '日期', dataIndex: 'date', search: false },
      { title: '周次', dataIndex: 'week', valueType: 'select', fieldProps: { options: weeks?.map(value => ({ value, label: '第' + value + '周' })) }, renderText: value => value },
      { title: '定稿版本', dataIndex: 'version', search: false },
      { title: '检查项', dataIndex: 'code', valueType: 'select', search: scope.code ? false : undefined, fieldProps: { options: columns?.filter(c => !c.section).map(c => ({ value: c.code, label: c.name })) }, render: (_, row) => columns?.find(c => c.code === row.code)?.name ?? row.code },
      { title: '原始量', search: false, render: (_, row) => row.amount === null ? '待核对' : row.amount + (columns?.find(c => c.code === row.code)?.unit ?? '') },
    ]} /> :
    <ProTable {...tableProps} dataSource={query.data?.cases} locale={{ emptyText: '本学期暂无处分事项' }} scroll={{ x: 'max-content' }} columns={[
      { title: '档位', dataIndex: 'tier', valueType: 'select', fieldProps: { options: query.data?.tiers.map(value => ({ value, label: value })) } },
      { title: '触发累计值', dataIndex: 'amount', search: false },
      { title: '来源定稿', search: false, render: (_, row) => `第${row.week}周 · V${row.version}` },
      { title: '状态', dataIndex: 'status', valueType: 'select', valueEnum: caseStates },
      { title: '处理截止', search: false, render: (_, row) => row.dueAt ? new Date(row.dueAt).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' }) : '—' },
    ]} />}</>
}
