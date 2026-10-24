import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Drawer, Input, Modal, Space, Table, Tag, Typography, message } from 'antd'
import { useRef, useState } from 'react'
import type { DepartmentSubmission, DepartmentSubmissionScope } from '../../../entities/inspection/model/departmentSubmission'
import { departmentSubmissionApi } from '../../../shared/api/departmentSubmissionApi'
import { ApiError } from '../../../shared/api/httpClient'
import { createIdempotencyKey } from '../../../shared/utils/idempotencyKey'

type SelectableRecord = DepartmentSubmissionScope & { id: string; rowVersion: number; status: string }
type Props = {
  inspectionTypeId: string
  listScope?: Partial<Record<'semesterId' | 'weekId' | 'departmentId', string | number | undefined>>
  selected: SelectableRecord[]
  canReview: boolean
  onChanged: () => void
  onOpenRecord?: (id: string, status: string) => void
}

const labels: Record<DepartmentSubmission['status'], string> = {
  PENDING_REVIEW: '待部门审核', RETURNED: '已退回', CONFIRMED: '部门已确认',
}

function oneScope(records: SelectableRecord[]): DepartmentSubmissionScope | undefined {
  const first = records[0]
  if (!first || records.some(item => item.status !== 'DRAFT' ||
    item.semesterId !== first.semesterId || item.weekId !== first.weekId ||
    item.departmentId !== first.departmentId || item.inspectionTypeId !== first.inspectionTypeId)) return undefined
  return { semesterId: first.semesterId, weekId: first.weekId, departmentId: first.departmentId, inspectionTypeId: first.inspectionTypeId }
}

/** Batch actions are shared by the five active inspection pages. The API enforces every scope and transition. */
export function DepartmentSubmissionActions({ inspectionTypeId, listScope, selected, canReview, onChanged, onOpenRecord }: Props) {
  const [open, setOpen] = useState(false)
  const [page, setPage] = useState(1)
  const [reasonFor, setReasonFor] = useState<DepartmentSubmission>()
  const [pendingSubmit, setPendingSubmit] = useState<{ scope: DepartmentSubmissionScope; records: { id: string; version: number }[]; key: string }>()
  const [decisionFor, setDecisionFor] = useState<{ item: DepartmentSubmission; kind: 'confirm' | 'resubmit' }>()
  const [reason, setReason] = useState('')
  const [notice, noticeContext] = message.useMessage()
  const queryClient = useQueryClient()
  const retryKey = useRef<{ signature: string; key: string }>(undefined)
  const scope = oneScope(selected)
  const queryScope = { semesterId: listScope?.semesterId, weekId: listScope?.weekId, departmentId: listScope?.departmentId }
  const list = useQuery({ queryKey: ['department-submissions', inspectionTypeId, queryScope, page],
    queryFn: () => departmentSubmissionApi.list({ inspectionTypeId, ...queryScope, page, pageSize: 10 }), enabled: open && !!inspectionTypeId })
  const action = useMutation({ mutationFn: async (input: { kind: 'submit'; scope: DepartmentSubmissionScope; records: { id: string; version: number }[]; key: string } | { kind: 'return' | 'confirm' | 'resubmit'; item: DepartmentSubmission; reason?: string }) => {
    if (input.kind === 'submit') return departmentSubmissionApi.submit(input.scope, input.records, input.key)
    if (input.kind === 'resubmit') return departmentSubmissionApi.resubmit(input.item.id, input.item.rowVersion, input.item.records.map(item => ({ id: item.id, version: item.rowVersion })))
    return departmentSubmissionApi.decide(input.item.id, input.kind, input.item.rowVersion, input.reason)
  }, onSuccess: async () => {
    retryKey.current = undefined
    setPendingSubmit(undefined)
    setDecisionFor(undefined)
    setReasonFor(undefined)
    setReason('')
    await Promise.all([queryClient.invalidateQueries({ queryKey: ['department-submissions'] }),
      queryClient.invalidateQueries({ queryKey: ['dashboard'] }), queryClient.invalidateQueries({ queryKey: ['morning-attendances'] }),
      queryClient.invalidateQueries({ queryKey: ['course-inspections'] }), queryClient.invalidateQueries({ queryKey: ['late-returns'] }),
      queryClient.invalidateQueries({ queryKey: ['dormitory-hygienes'] }), queryClient.invalidateQueries({ queryKey: ['attendance'] })])
    onChanged()
    void notice.success('批次状态已更新')
  }, onError: error => {
    void Promise.all([queryClient.invalidateQueries({ queryKey: ['department-submissions'] }),
      queryClient.invalidateQueries({ queryKey: ['morning-attendances'] }), queryClient.invalidateQueries({ queryKey: ['course-inspections'] }),
      queryClient.invalidateQueries({ queryKey: ['late-returns'] }), queryClient.invalidateQueries({ queryKey: ['dormitory-hygienes'] }),
      queryClient.invalidateQueries({ queryKey: ['attendance'] })])
    void notice.error(error instanceof ApiError ? error.message : '操作失败，请刷新记录或批次后重试')
  } })

  const submit = () => {
  if (!scope) { void notice.warning('请选择同一学期、周次、归属和类型下的草稿记录'); return }
    const signature = selected.map(item => `${item.id}:${item.rowVersion}`).sort().join('|')
    if (retryKey.current?.signature !== signature) retryKey.current = { signature, key: createIdempotencyKey() }
    setPendingSubmit({ scope, records: selected.map(item => ({ id: item.id, version: item.rowVersion })), key: retryKey.current.key })
  }
  const decide = (item: DepartmentSubmission, kind: 'confirm' | 'resubmit') => {
    setDecisionFor({ item, kind })
  }

  return <>
    {noticeContext}
    <Button disabled={!selected.length || action.isPending} onClick={submit}>提交所选草稿（{selected.length}）</Button>
    <Button disabled={!inspectionTypeId} onClick={() => setOpen(true)}>检查提交批次</Button>
    <Modal title="确认提交所选记录？" open={!!pendingSubmit} confirmLoading={action.isPending} okText="确认提交"
      onCancel={() => setPendingSubmit(undefined)} onOk={() => { if (pendingSubmit) action.mutate({ kind: 'submit', ...pendingSubmit }) }}>
      本次共 {pendingSubmit?.records.length ?? 0} 条记录，提交后等待授权审核人审核。
    </Modal>
    <Modal title={decisionFor?.kind === 'confirm' ? '确认该批次？' : '重新提交该批次？'} open={!!decisionFor}
      confirmLoading={action.isPending} okText={decisionFor?.kind === 'confirm' ? '确认批次' : '重新提交'}
      onCancel={() => setDecisionFor(undefined)} onOk={() => { if (decisionFor) action.mutate(decisionFor) }}>
      {decisionFor?.kind === 'confirm' ? `将确认这 ${decisionFor.item.recordIds.length} 条记录并纳入后续通报。` : '将按当前记录版本重新提交整个原批次。'}
    </Modal>
    <Drawer title="检查提交批次" open={open} onClose={() => setOpen(false)} width={760} destroyOnHidden>
      <Typography.Paragraph type="secondary">按批次提交，部门与校级归属分别按周汇总。退回后可更正记录，也可原样在原批次重提。</Typography.Paragraph>
      {list.isError && <Alert type="error" title="批次加载失败" action={<Button onClick={() => void list.refetch()}>重试</Button>} />}
      <Table rowKey="id" loading={list.isLoading} dataSource={list.isError ? [] : list.data?.items ?? []}
        locale={{ emptyText: list.isError ? '批次加载失败，请重试' : '当前范围暂无提交批次' }}
        pagination={{ current: page, pageSize: 10, total: list.data?.total ?? 0, onChange: setPage }}
        scroll={{ x: 650 }} columns={[
          { title: '周次范围', key: 'scope', render: (_, item) => <>{item.semesterName ?? '学期'}{item.weekNo ? ` · 第 ${item.weekNo} 周` : ''}<br />{item.departmentName ?? '校级'}</> },
          { title: '记录数', key: 'count', render: (_, item) => item.recordIds.length },
          { title: '修订', dataIndex: 'revisionNo' },
          { title: '状态', key: 'status', render: (_, item) => <Tag>{item.scope.departmentId ? labels[item.status] : item.status === 'PENDING_REVIEW' ? '待校级审核' : item.status === 'CONFIRMED' ? '校级已确认' : labels[item.status]}</Tag> },
          { title: '操作', key: 'actions', render: (_, item) => <Space wrap>
            {item.status === 'PENDING_REVIEW' && canReview && <><Button type="link" onClick={() => { setReasonFor(item); setReason('') }}>退回</Button><Button type="link" onClick={() => decide(item, 'confirm')}>确认</Button></>}
            {item.status === 'RETURNED' && <Button type="link" onClick={() => decide(item, 'resubmit')}>重提</Button>}
          </Space> },
        ]} expandable={{ expandedRowRender: item => <Space direction="vertical"><Typography.Text>提交时间：{new Date(item.submittedAt).toLocaleString('zh-CN')}</Typography.Text><Typography.Text>记录数：{item.recordIds.length}</Typography.Text>{item.returnReason && <Alert type="warning" title={`退回原因：${item.returnReason}`} />}{onOpenRecord && item.records.map((record, index) => record.status === 'RETURNED' && <Button key={record.id} type="link" onClick={() => { setOpen(false); onOpenRecord(record.id, record.status) }}>更正第 {index + 1} 条记录</Button>)}</Space> }} />
    </Drawer>
    <Modal title="退回提交批次" open={!!reasonFor} okText="确认退回" okButtonProps={{ disabled: !reason.trim() || action.isPending }}
      confirmLoading={action.isPending} onCancel={() => setReasonFor(undefined)} onOk={() => { if (reasonFor) action.mutate({ kind: 'return', item: reasonFor, reason: reason.trim() }) }}>
      <Typography.Paragraph>请说明需要更正的内容。退回会影响整个批次的 {reasonFor?.recordIds.length ?? 0} 条记录。</Typography.Paragraph>
      <Input.TextArea aria-label="退回原因" value={reason} maxLength={500} onChange={event => setReason(event.target.value)} rows={3} />
    </Modal>
  </>
}
