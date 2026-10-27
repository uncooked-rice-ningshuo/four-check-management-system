import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Drawer, Input, Modal, Space, Table, Tag, Typography, message } from 'antd'
import { useRef, useState } from 'react'
import type { DepartmentSubmission, DepartmentSubmissionKind, DepartmentSubmissionScope } from '../../../entities/inspection/model/departmentSubmission'
import type { SubmissionCartBatchRecord, SubmissionCartRecord } from '../../../entities/inspection/model/submissionCart'
import { departmentSubmissionApi } from '../../../shared/api/departmentSubmissionApi'
import { submissionCartApi } from '../../../shared/api/submissionCartApi'
import { ApiError } from '../../../shared/api/httpClient'
import { createIdempotencyKey } from '../../../shared/utils/idempotencyKey'
import { COLLEGE_DEPARTMENT_OPTION_ID } from '../../../entities/inspection/model/inspectionScope'
import { inspectionTypeLabel } from '../../../entities/inspection/model/types'
import { recordSourceLabel, recordStatusLabel } from '../../../entities/inspection/model/recordLabels'

type SelectableRecord = DepartmentSubmissionScope & { id: string; rowVersion: number; status: string; source?: string; studentNumber?: string; studentName?: string; className?: string; grade?: string; typeCode?: string; importBatchId?: string }
type ScopeOption = { id: string; label: string }
/** Optional readable names for the scope ids; without them the drawer must never print raw UUIDs. */
type ScopeLabels = { semesters?: ScopeOption[]; weeks?: ScopeOption[]; departments?: ScopeOption[]; inspectionTypeName?: string }
type Props = {
  inspectionTypeId: string
  listScope?: Partial<Record<'semesterId' | 'weekId' | 'departmentId', string | number | undefined>>
  selected: SelectableRecord[]
  canReview: boolean
  scopeLabels?: ScopeLabels
  onChanged: () => void
  onOpenRecord?: (id: string, status: string) => void
}

const labels: Record<DepartmentSubmission['status'], string> = {
  PENDING_REVIEW: '待部门审核', RETURNED: '已退回', CONFIRMED: '部门已确认',
}

function optionLabel(options: ScopeOption[] | undefined, id: string) {
  return options?.find(item => item.id === id)?.label
}

/** Readable scope text; raw ids must never reach the confirmation dialogs. */
function scopeText(target: DepartmentSubmissionScope, scopeLabels?: ScopeLabels) {
  return [
    optionLabel(scopeLabels?.semesters, target.semesterId) ?? '当前学期',
    optionLabel(scopeLabels?.weeks, target.weekId) ?? '当前周次',
    target.departmentId ? optionLabel(scopeLabels?.departments, target.departmentId) ?? '所属部门' : '校级',
    scopeLabels?.inspectionTypeName ?? '当前检查类型',
  ].join(' · ')
}

function oneScope(records: SelectableRecord[]): DepartmentSubmissionScope | undefined {
  const first = records[0]
  if (!first || records.some(item => item.status !== 'DRAFT' ||
    item.semesterId !== first.semesterId || item.weekId !== first.weekId ||
    item.departmentId !== first.departmentId || item.inspectionTypeId !== first.inspectionTypeId)) return undefined
  return { semesterId: first.semesterId, weekId: first.weekId, departmentId: first.departmentId, inspectionTypeId: first.inspectionTypeId }
}

function explicitReceiptScope(listScope: Props['listScope'], inspectionTypeId: string): DepartmentSubmissionScope | undefined {
  const semesterId = listScope?.semesterId
  const weekId = listScope?.weekId
  const departmentId = listScope?.departmentId
  if (typeof semesterId !== 'string' || !semesterId.trim() || typeof weekId !== 'string' || !weekId.trim() ||
    typeof departmentId !== 'string' || !departmentId.trim() || !inspectionTypeId.trim()) return undefined
  return { semesterId, weekId, departmentId: departmentId === COLLEGE_DEPARTMENT_OPTION_ID ? '' : departmentId, inspectionTypeId }
}

/** Batch actions are shared by the active inspection pages. The API enforces every scope and transition. */
export function DepartmentSubmissionActions({ inspectionTypeId, listScope, selected, canReview, scopeLabels, onChanged, onOpenRecord }: Props) {
  const [open, setOpen] = useState(false)
  const [page, setPage] = useState(1)
  const [reasonFor, setReasonFor] = useState<DepartmentSubmission>()
  const [pendingSubmit, setPendingSubmit] = useState<{ scope: DepartmentSubmissionScope; records: { id: string; version: number }[]; key: string; submissionKind: DepartmentSubmissionKind }>()
  const [decisionFor, setDecisionFor] = useState<{ item: DepartmentSubmission; kind: 'confirm' | 'resubmit' }>()
  const [reason, setReason] = useState('')
  const [cartOpen, setCartOpen] = useState(false)
  const [cart, setCart] = useState<Map<string, SubmissionCartRecord>>(() => new Map())
  const [candidatePage, setCandidatePage] = useState(1)
  const [importPage, setImportPage] = useState(1)
  const [activeImportBatch, setActiveImportBatch] = useState<string>()
  const [batchRecordPage, setBatchRecordPage] = useState(1)
  const [candidateKeyword, setCandidateKeyword] = useState('')
  const [candidateSearch, setCandidateSearch] = useState('')
  const [staleRecords, setStaleRecords] = useState<{ id: string; reason: string }[]>([])
  const [cartScopeSnapshot, setCartScopeSnapshot] = useState('')
  const [notice, noticeContext] = message.useMessage()
  const queryClient = useQueryClient()
  const retryKey = useRef<{ signature: string; key: string }>(undefined)
  const scope = oneScope(selected)
  const receiptScope = explicitReceiptScope(listScope, inspectionTypeId)
  const cartScope = receiptScope ?? scope
  const cartScopeKey = cartScope ? `${cartScope.semesterId}:${cartScope.weekId}:${cartScope.departmentId}:${cartScope.inspectionTypeId}` : ''
  const contextChanged = cart.size > 0 && cartScopeSnapshot !== cartScopeKey
  const scopeQuery = cartScope ? {
    semesterId: cartScope.semesterId,
    weekId: cartScope.weekId,
    inspectionTypeId: cartScope.inspectionTypeId,
    departmentId: cartScope.departmentId || COLLEGE_DEPARTMENT_OPTION_ID,
  } : undefined
  const candidates = useQuery({
    queryKey: ['submission-cart', 'candidates', scopeQuery, candidatePage, candidateKeyword],
    queryFn: () => submissionCartApi.candidates({ ...scopeQuery, keyword: candidateKeyword, page: candidatePage, pageSize: 10 }),
    enabled: cartOpen && !!scopeQuery,
  })
  const importBatches = useQuery({
    queryKey: ['submission-cart', 'imports', scopeQuery, importPage],
    queryFn: () => submissionCartApi.batches({ ...scopeQuery, page: importPage, pageSize: 10 }),
    enabled: cartOpen && !!scopeQuery,
  })
  const batchRecords = useQuery({
    queryKey: ['submission-cart', 'import-records', activeImportBatch, batchRecordPage],
    queryFn: () => submissionCartApi.batchRecords(activeImportBatch ?? '', { page: batchRecordPage, pageSize: 10 }),
    enabled: cartOpen && !!activeImportBatch,
  })
  const queryScope = { semesterId: listScope?.semesterId, weekId: listScope?.weekId, departmentId: listScope?.departmentId }
  const list = useQuery({ queryKey: ['department-submissions', inspectionTypeId, queryScope, page],
    queryFn: () => departmentSubmissionApi.list({ inspectionTypeId, ...queryScope, page, pageSize: 10 }), enabled: open && !!inspectionTypeId })
  const action = useMutation({ mutationFn: async (input: { kind: 'submit'; scope: DepartmentSubmissionScope; records: { id: string; version: number }[]; key: string; submissionKind: DepartmentSubmissionKind; explanation?: string } | { kind: 'return' | 'confirm' | 'resubmit'; item: DepartmentSubmission; reason?: string; explanation?: string }) => {
    if (input.kind === 'submit') return input.submissionKind === 'RECORDS'
      ? departmentSubmissionApi.submit(input.scope, input.records, input.key)
      : departmentSubmissionApi.submit(input.scope, input.records, input.key, input.submissionKind, input.explanation)
    if (input.kind === 'resubmit') {
      const records = input.item.records.map(item => ({ id: item.id, version: item.rowVersion }))
      return input.item.kind === 'ZERO_RECORD'
        ? departmentSubmissionApi.resubmit(input.item.id, input.item.rowVersion, records, 'ZERO_RECORD', input.explanation)
        : departmentSubmissionApi.resubmit(input.item.id, input.item.rowVersion, records)
    }
    return departmentSubmissionApi.decide(input.item.id, input.kind, input.item.rowVersion, input.reason)
  }, onSuccess: async (result, variables) => {
    retryKey.current = undefined
    setPendingSubmit(undefined)
    setDecisionFor(undefined)
    setReasonFor(undefined)
    setReason('')
    setStaleRecords([])
    if (variables.kind === 'submit' && variables.submissionKind === 'RECORDS') {
      setCart(new Map())
      setCartOpen(false)
    }
    await Promise.all([queryClient.invalidateQueries({ queryKey: ['department-submissions'] }),
      queryClient.invalidateQueries({ queryKey: ['submission-cart'] }),
      queryClient.invalidateQueries({ queryKey: ['reports', 'summary'] }), queryClient.invalidateQueries({ queryKey: ['reports', 'records'] }),
      queryClient.invalidateQueries({ queryKey: ['dashboard'] }), queryClient.invalidateQueries({ queryKey: ['morning-attendances'] }),
      queryClient.invalidateQueries({ queryKey: ['course-inspections'] }), queryClient.invalidateQueries({ queryKey: ['late-returns'] }),
      queryClient.invalidateQueries({ queryKey: ['dormitory-hygienes'] }), queryClient.invalidateQueries({ queryKey: ['evening-self-studies'] }), queryClient.invalidateQueries({ queryKey: ['attendance'] })])
    onChanged()
    void notice.success(`${variables.kind === 'submit' ? '已创建提交批次' : '批次状态已更新'}：${result.id}（${labels[result.status as DepartmentSubmission['status']] ?? result.status}）`)
  }, onError: error => {
    void Promise.all([queryClient.invalidateQueries({ queryKey: ['department-submissions'] }),
      queryClient.invalidateQueries({ queryKey: ['submission-cart'] }),
      queryClient.invalidateQueries({ queryKey: ['reports', 'summary'] }), queryClient.invalidateQueries({ queryKey: ['reports', 'records'] }),
      queryClient.invalidateQueries({ queryKey: ['morning-attendances'] }), queryClient.invalidateQueries({ queryKey: ['course-inspections'] }),
      queryClient.invalidateQueries({ queryKey: ['late-returns'] }), queryClient.invalidateQueries({ queryKey: ['dormitory-hygienes'] }),
      queryClient.invalidateQueries({ queryKey: ['evening-self-studies'] }), queryClient.invalidateQueries({ queryKey: ['attendance'] })])
    void notice.error(error instanceof ApiError ? error.message : '操作失败，请刷新记录或批次后重试')
  } })

  const openCart = () => {
    if (!cartScope) {
      void notice.warning('请先在列表中选择明确的学期、周次和部门/校级范围')
      return
    }
    if (contextChanged) {
      setCartOpen(true)
      void notice.warning('业务范围已变化，请先清空旧清单后再继续')
      return
    }
    setCartScopeSnapshot(cartScopeKey)
    setStaleRecords([])
    setCart(current => {
      const next = new Map(current)
      for (const item of selected.filter(record => record.status === 'DRAFT' && record.semesterId === cartScope.semesterId && record.weekId === cartScope.weekId && record.departmentId === cartScope.departmentId && record.inspectionTypeId === cartScope.inspectionTypeId)) {
        next.set(item.id, {
          id: item.id, semesterId: item.semesterId, weekId: item.weekId, inspectionTypeId: item.inspectionTypeId,
          departmentId: item.departmentId, typeCode: item.typeCode ?? '', studentNumber: item.studentNumber ?? '',
          studentName: item.studentName ?? '', className: item.className ?? '', grade: item.grade ?? '',
          status: item.status, source: item.source ?? 'MANUAL', importBatchId: item.importBatchId,
          rowVersion: item.rowVersion, canEdit: true, details: {},
        })
      }
      return next
    })
    setCartOpen(true)
  }

  const toggleCartRecord = (record: SubmissionCartRecord, add: boolean) => {
    if (contextChanged) return
    setCart(current => {
      const next = new Map(current)
      if (add) next.set(record.id, record)
      else next.delete(record.id)
      return next
    })
  }

  const verifyCart = async () => {
    if (!cartScope || !cart.size || contextChanged) return
    setStaleRecords([])
    const checks = await Promise.all([...cart.values()].map(async record => {
      try {
        const current = await submissionCartApi.detail(record.id)
        if (current.rowVersion !== record.rowVersion) return { id: record.id, reason: '记录已被修改，请重新加载后选择' }
        if (current.status !== 'DRAFT') return { id: record.id, reason: '记录已进入原提交批次，仅可从原批次重提' }
        if (!current.canEdit) return { id: record.id, reason: '当前权限或通报锁定状态已变化' }
        return undefined
      } catch (error) {
        return { id: record.id, reason: error instanceof ApiError && error.status === 404 ? '记录已删除或不可见' : '当前无法读取记录，请重试' }
      }
    }))
    const stale = checks.filter((item): item is { id: string; reason: string } => !!item)
    if (stale.length) {
      setStaleRecords(stale)
      return
    }
    const records = [...cart.values()].map(item => ({ id: item.id, version: item.rowVersion }))
    const signature = `RECORDS:${cartScope.semesterId}:${cartScope.weekId}:${cartScope.departmentId}:${cartScope.inspectionTypeId}:${records.map(item => `${item.id}:${item.version}`).sort().join('|')}:`
    if (retryKey.current?.signature !== signature) retryKey.current = { signature, key: createIdempotencyKey() }
    setPendingSubmit({ scope: cartScope, records, key: retryKey.current.key, submissionKind: 'RECORDS' })
  }

  const submitZeroReceipt = () => {
    if (!receiptScope) return
    const signature = `ZERO_RECORD:${receiptScope.semesterId}:${receiptScope.weekId}:${receiptScope.departmentId}:${receiptScope.inspectionTypeId}::`
    if (retryKey.current?.signature !== signature) retryKey.current = { signature, key: createIdempotencyKey() }
    setReason('')
    setPendingSubmit({ scope: receiptScope, records: [], key: retryKey.current.key, submissionKind: 'ZERO_RECORD' })
  }
  const decide = (item: DepartmentSubmission, kind: 'confirm' | 'resubmit') => {
    if (kind === 'resubmit') setReason('')
    setDecisionFor({ item, kind })
  }
  const confirmSubmit = () => {
    if (!pendingSubmit) return
    const explanation = pendingSubmit.submissionKind === 'ZERO_RECORD' ? reason.trim() : ''
    const signature = `${pendingSubmit.submissionKind}:${pendingSubmit.scope.semesterId}:${pendingSubmit.scope.weekId}:${pendingSubmit.scope.departmentId}:${pendingSubmit.scope.inspectionTypeId}:${pendingSubmit.records.map(item => `${item.id}:${item.version}`).sort().join('|')}:${explanation}`
    if (retryKey.current?.signature !== signature) retryKey.current = { signature, key: createIdempotencyKey() }
    action.mutate({ kind: 'submit', ...pendingSubmit, key: retryKey.current.key, explanation })
  }

  const addWholeBatch = async (batchId: string) => {
    if (contextChanged) return
    try {
      const firstPage = await submissionCartApi.batchRecords(batchId, { page: 1, pageSize: 100 })
      const all = [...firstPage.items]
      for (let pageNumber = 2; (pageNumber - 1) * firstPage.pageSize < firstPage.total; pageNumber++) {
        const nextPage = await submissionCartApi.batchRecords(batchId, { page: pageNumber, pageSize: firstPage.pageSize })
        all.push(...nextPage.items)
      }
      setCart(current => {
        const next = new Map(current)
        for (const item of all) if (item.selectable) next.set(item.record.id, item.record)
        return next
      })
      setActiveImportBatch(batchId)
      void notice.success(`已添加 ${all.filter(item => item.selectable).length} 条可提交记录`)
    } catch (error) {
      void notice.error(error instanceof ApiError ? error.message : '导入批次记录加载失败，请重试')
    }
  }

  const removeCartRecord = (id: string) => setCart(current => {
    const next = new Map(current)
    next.delete(id)
    return next
  })

  const recordLabel = (record: SubmissionCartRecord) => {
    const people = [record.studentName, record.studentNumber, record.className, record.grade].filter(Boolean).join(' · ')
    if (people) return people
    // Object-level inspections carry no student snapshot; use the live reference name instead of the raw id.
    const reference = record as SubmissionCartRecord & Record<string, unknown>
    const subject = ['currentDormitoryName', 'currentClassroomName', 'currentAreaName', 'currentCourseName']
      .map(key => reference[key]).find(value => typeof value === 'string' && value.trim()) as string | undefined
    const date = typeof record.details?.date === 'string' ? record.details.date : undefined
    return [subject, date].filter(Boolean).join(' · ') || '（无名称记录）'
  }

  return <>
    {noticeContext}
    <Button disabled={!cartScope || action.isPending} onClick={openCart}>准备提交{cart.size ? `（${cart.size}）` : selected.length ? `（已选 ${selected.length}）` : ''}</Button>
    {receiptScope && <Button disabled={action.isPending} onClick={submitZeroReceipt}>提交零记录回执</Button>}
    <Button disabled={!inspectionTypeId} onClick={() => setOpen(true)}>检查提交批次</Button>
    <Modal title={pendingSubmit?.submissionKind === 'ZERO_RECORD' ? '确认提交零记录回执？' : '最终确认本次提交'} open={!!pendingSubmit} confirmLoading={action.isPending} okText="确认提交"
      onCancel={() => setPendingSubmit(undefined)} onOk={confirmSubmit}>
      {pendingSubmit?.submissionKind === 'ZERO_RECORD'
        ? <><Typography.Paragraph>将提交所选部门或校级范围在当前学期、周次和检查类型下“已检查、确认零条”的回执。提交后进入现有审核流程。</Typography.Paragraph><Input.TextArea aria-label="零记录说明（可选）" placeholder="说明可选" value={reason} maxLength={500} onChange={event => setReason(event.target.value)} rows={3} /></>
        : <><Typography.Paragraph>范围：{pendingSubmit ? scopeText(pendingSubmit.scope, scopeLabels) : ''}；共 {pendingSubmit?.records.length ?? 0} 条。请确认本次无遗漏和多选，提交后进入授权审核人的批次审核。</Typography.Paragraph><Table size="small" rowKey="id" pagination={false} dataSource={[...cart.values()]} scroll={{ x: 680, y: 280 }} columns={[{ title: '学生/事项', key: 'record', render: (_, item) => recordLabel(item) }, { title: '来源', dataIndex: 'source', render: value => recordSourceLabel(value as string) }, { title: '导入批次/行', key: 'import', render: (_, item) => item.source === 'EXCEL' ? `${item.importBatchId ? `批次 ${item.importBatchId.slice(0, 8)}` : 'Excel 批次'}${item.importRowNo ? ` / 第 ${item.importRowNo} 行` : ''}` : '—' }]} /><Typography.Paragraph type="secondary">点击“确认提交”后仍会由服务端重新验证每条记录的权限、状态和版本。</Typography.Paragraph></>}
    </Modal>
    <Modal title={decisionFor?.kind === 'confirm' ? '确认该批次？' : '重新提交该批次？'} open={!!decisionFor}
      confirmLoading={action.isPending} okText={decisionFor?.kind === 'confirm' ? '确认批次' : '重新提交'}
      onCancel={() => setDecisionFor(undefined)} onOk={() => { if (decisionFor) action.mutate({ ...decisionFor, explanation: decisionFor.kind === 'resubmit' && decisionFor.item.kind === 'ZERO_RECORD' ? reason.trim() : undefined }) }}>
      {decisionFor?.kind === 'confirm' ? `将确认${decisionFor.item.kind === 'ZERO_RECORD' ? '“已检查、确认零条”回执' : `这 ${decisionFor.item.recordIds.length} 条记录`}并纳入后续通报。` : decisionFor?.item.kind === 'ZERO_RECORD' ? <><Typography.Paragraph>将保留原回执并追加新修订，重新进入审核流程。说明可选。</Typography.Paragraph><Input.TextArea aria-label="零记录重提说明（可选）" value={reason} maxLength={500} onChange={event => setReason(event.target.value)} rows={3} /></> : '将按当前记录版本重新提交整个原批次。'}
    </Modal>
    <Drawer title="检查提交批次" open={open} onClose={() => setOpen(false)} width={760} destroyOnHidden>
      <Typography.Paragraph type="secondary">按批次提交，部门与校级归属分别按周汇总。退回后可更正记录，也可原样在原批次重提。</Typography.Paragraph>
      {list.isError && <Alert type="error" title="批次加载失败" action={<Button onClick={() => void list.refetch()}>重试</Button>} />}
      <Table rowKey="id" loading={list.isLoading} dataSource={list.isError ? [] : list.data?.items ?? []}
        locale={{ emptyText: list.isError ? '批次加载失败，请重试' : '当前范围暂无提交批次' }}
        pagination={{ current: page, pageSize: 10, total: list.data?.total ?? 0, onChange: setPage }}
        scroll={{ x: 650 }} columns={[
          { title: '周次范围', key: 'scope', render: (_, item) => <>{item.semesterName ?? '学期'}{item.weekNo ? ` · 第 ${item.weekNo} 周` : ''}<br />{item.departmentName ?? '校级'}</> },
          { title: '记录数', key: 'count', render: (_, item) => item.kind === 'ZERO_RECORD' ? <Tag color="blue">确认零条</Tag> : item.recordIds.length },
          { title: '修订', dataIndex: 'revisionNo' },
          { title: '状态', key: 'status', render: (_, item) => <Tag>{item.scope.departmentId ? labels[item.status] ?? item.status : item.status === 'PENDING_REVIEW' ? '待校级审核' : item.status === 'CONFIRMED' ? '校级已确认' : labels[item.status] ?? item.status}</Tag> },
          { title: '操作', key: 'actions', render: (_, item) => <Space wrap>
            {item.status === 'PENDING_REVIEW' && canReview && <><Button type="link" onClick={() => { setReasonFor(item); setReason('') }}>退回</Button><Button type="link" onClick={() => decide(item, 'confirm')}>确认</Button></>}
            {item.status === 'RETURNED' && <Button type="link" onClick={() => decide(item, 'resubmit')}>重提</Button>}
          </Space> },
        ]} expandable={{ expandedRowRender: item => <Space direction="vertical"><Typography.Text>提交时间：{new Date(item.submittedAt).toLocaleString('zh-CN')}</Typography.Text><Typography.Text>{item.kind === 'ZERO_RECORD' ? '已检查并确认零条；该回执不包含记录。后续发现实际记录时，应另行提交新批次。' : `记录数：${item.recordIds.length}`}</Typography.Text>{item.returnReason && <Alert type="warning" title={`退回原因：${item.returnReason}`} />}{onOpenRecord && item.records.map((record, index) => record.status === 'RETURNED' && <Button key={record.id} type="link" onClick={() => { setOpen(false); onOpenRecord(record.id, record.status) }}>更正第 {index + 1} 条记录</Button>)}</Space> }} />
    </Drawer>
    <Drawer title="准备提交" open={cartOpen} width={980} destroyOnHidden onClose={() => {
      if (cart.size) Modal.confirm({ title: '关闭并丢弃待提交清单？', content: `当前清单有 ${cart.size} 条记录，关闭后会清空本次选择。`, okText: '丢弃并关闭', cancelText: '继续核对', onOk: () => { setCart(new Map()); setCartOpen(false); setStaleRecords([]) } })
      else setCartOpen(false)
    }}>
      {cartScope && <Typography.Paragraph strong>范围：{scopeText(cartScope, scopeLabels)}</Typography.Paragraph>}
      {contextChanged && <Alert type="warning" showIcon title="业务范围已变化。清空旧清单后才能在新范围选择记录。" action={<Button onClick={() => { setCart(new Map()); setCartScopeSnapshot(cartScopeKey); setStaleRecords([]) }}>清空并切换范围</Button>} />}
      <Typography.Paragraph>待提交 {cart.size} 条；当前范围仍可首次提交 {candidates.data?.total ?? '—'} 条。以下候选按服务端权限和当前记录状态筛选。</Typography.Paragraph>
      {candidates.isError && <Alert type="error" title="候选记录加载失败" action={<Button onClick={() => void candidates.refetch()}>重试</Button>} />}
      <Typography.Title level={5}>从记录添加</Typography.Title>
      <Input.Search aria-label="检索候选记录" placeholder="姓名、学号、班级或备注" allowClear value={candidateSearch} onChange={event => setCandidateSearch(event.target.value)} onSearch={value => { setCandidateKeyword(value.trim()); setCandidatePage(1) }} style={{ maxWidth: 360, marginBottom: 8 }} />
      <Table<SubmissionCartRecord> size="small" rowKey="id" loading={candidates.isLoading} dataSource={candidates.isError ? [] : candidates.data?.items ?? []}
        locale={{ emptyText: candidates.isError ? '候选记录加载失败，请重试' : '当前范围暂无可提交记录' }}
        rowSelection={{ selectedRowKeys: candidates.data?.items.filter(item => cart.has(item.id)).map(item => item.id), getCheckboxProps: () => ({ disabled: contextChanged }), onSelect: (item, checked) => toggleCartRecord(item, checked), onSelectAll: (checked, rows) => { if (checked) rows.forEach(item => toggleCartRecord(item, true)); else candidates.data?.items.forEach(item => toggleCartRecord(item, false)) } }}
        pagination={{ current: candidatePage, pageSize: 10, total: candidates.data?.total ?? 0, onChange: setCandidatePage }} scroll={{ x: 640 }}
        columns={[{ title: '学生/事项', key: 'student', render: (_, item) => recordLabel(item) }, { title: '类型', key: 'typeCode', render: (_, item) => inspectionTypeLabel(item.typeCode) }, { title: '来源', dataIndex: 'source', render: value => recordSourceLabel(value as string) }, { title: '状态', key: 'status', render: (_, item) => recordStatusLabel(item.status) }]} />
      <Typography.Title level={5}>从 Excel 批次添加</Typography.Title>
      {importBatches.isError && <Alert type="error" title="本人历史导入批次加载失败" action={<Button onClick={() => void importBatches.refetch()}>重试</Button>} />}
      <Table size="small" rowKey="id" loading={importBatches.isLoading} dataSource={importBatches.isError ? [] : importBatches.data?.items ?? []} locale={{ emptyText: importBatches.isError ? '批次加载失败，请重试' : '当前范围暂无已确认入库的 Excel 批次' }}
        pagination={{ current: importPage, pageSize: 10, total: importBatches.data?.total ?? 0, onChange: setImportPage }} scroll={{ x: 880 }} columns={[
          { title: '文件', dataIndex: 'originalName' }, { title: '确认时间', dataIndex: 'confirmedAt', render: value => value === '0001-01-01T00:00:00Z' ? '—' : new Date(value).toLocaleString('zh-CN') },
          { title: '入库数', dataIndex: 'importedRows' }, { title: '可提交', dataIndex: 'selectableRecords' }, { title: '已提交', dataIndex: 'submittedRecords' }, { title: '不可用', dataIndex: 'unavailableRecords' },
          { title: '操作', key: 'actions', render: (_, item) => <Space><Button type="link" onClick={() => { setActiveImportBatch(item.id); setBatchRecordPage(1) }}>查看记录</Button><Button type="link" disabled={contextChanged || !item.selectableRecords} onClick={() => void addWholeBatch(item.id)}>添加整批可提交记录</Button></Space> },
        ]} />
      {activeImportBatch && <><Typography.Title level={5}>批次内现存业务记录</Typography.Title>{batchRecords.isError && <Alert type="error" title="批次记录加载失败" action={<Button onClick={() => void batchRecords.refetch()}>重试</Button>} />}<Table<SubmissionCartBatchRecord> size="small" rowKey={item => item.record.id} loading={batchRecords.isLoading} dataSource={batchRecords.isError ? [] : batchRecords.data?.items ?? []} locale={{ emptyText: batchRecords.isError ? '批次记录加载失败，请重试' : '该批次当前没有可显示的业务记录' }}
        rowSelection={{ selectedRowKeys: batchRecords.data?.items.filter(item => cart.has(item.record.id)).map(item => item.record.id), getCheckboxProps: item => ({ disabled: contextChanged || !item.selectable }), onSelect: (item, checked) => toggleCartRecord(item.record, checked), onSelectAll: (checked, rows) => { if (checked) rows.filter(item => item.selectable).forEach(item => toggleCartRecord(item.record, true)); else batchRecords.data?.items.forEach(item => toggleCartRecord(item.record, false)) } }}
        pagination={{ current: batchRecordPage, pageSize: 10, total: batchRecords.data?.total ?? 0, onChange: setBatchRecordPage }} scroll={{ x: 780 }} columns={[{ title: 'Excel 行', dataIndex: ['record', 'importRowNo'], render: value => value ? `第 ${value} 行` : '—' }, { title: '学生/事项', key: 'record', render: (_, item) => recordLabel(item.record) }, { title: '状态', key: 'status', render: (_, item) => recordStatusLabel(item.record.status) }, { title: '可选状态', key: 'availability', render: (_, item) => item.selectable ? <Tag color="green">可提交</Tag> : item.unavailableReason === 'ALREADY_SUBMITTED' ? '已属原提交批次，仅可从原批次重提' : '当前不可提交' }]} />
      </>}
      {staleRecords.length > 0 && <Alert type="warning" showIcon title="部分记录已失效，请移除后重新添加" description={staleRecords.map(item => `${recordLabel(cart.get(item.id) ?? { id: item.id } as SubmissionCartRecord)}：${item.reason}`).join('；')} />}
      <Typography.Title level={5}>统一待提交清单</Typography.Title>
      <Space style={{ marginBottom: 8 }}><Button onClick={() => { setCart(new Map()); setCartScopeSnapshot(cartScopeKey); setStaleRecords([]) }} disabled={!cart.size}>清空清单</Button><Typography.Text type="secondary">移除清单项不会修改业务记录。</Typography.Text></Space>
      <Table<SubmissionCartRecord> size="small" rowKey="id" dataSource={[...cart.values()]} pagination={{ pageSize: 8 }} scroll={{ x: 760 }} columns={[{ title: '学生/事项', key: 'record', render: (_, item) => recordLabel(item) }, { title: '来源', dataIndex: 'source', render: value => recordSourceLabel(value as string) }, { title: '导入批次/行', key: 'import', render: (_, item) => item.source === 'EXCEL' ? `${item.importBatchId ? `批次 ${item.importBatchId.slice(0, 8)}` : 'Excel 批次'}${item.importRowNo ? ` / 第 ${item.importRowNo} 行` : ''}` : '—' }, { title: '操作', key: 'remove', render: (_, item) => <Button type="link" danger onClick={() => removeCartRecord(item.id)}>移除</Button> }]} />
      <Button type="primary" disabled={!cart.size || !cartScope} loading={action.isPending} onClick={() => void verifyCart()}>核对并提交</Button>
    </Drawer>
    <Modal title="退回提交批次" open={!!reasonFor} okText="确认退回" okButtonProps={{ disabled: !reason.trim() || action.isPending }}
      confirmLoading={action.isPending} onCancel={() => setReasonFor(undefined)} onOk={() => { if (reasonFor) action.mutate({ kind: 'return', item: reasonFor, reason: reason.trim() }) }}>
      <Typography.Paragraph>请说明需要更正的内容。退回会影响{reasonFor?.kind === 'ZERO_RECORD' ? '整个零记录回执' : `整个批次的 ${reasonFor?.recordIds.length ?? 0} 条记录`}。</Typography.Paragraph>
      <Input.TextArea aria-label="退回原因" value={reason} maxLength={500} onChange={event => setReason(event.target.value)} rows={3} />
    </Modal>
  </>
}
