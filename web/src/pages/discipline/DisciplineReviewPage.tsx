import { useState } from 'react'
import { Alert, Button, Form, Input, Modal, Space, Tag, Typography, Empty } from 'antd'
import { ProTable } from '@ant-design/pro-components'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { searchReviews, submitReview, type ReviewItem, type ReviewFilter } from '../../shared/api/disciplineReviewApi'
import { TableRowLegend } from '../../shared/components/TableRowLegend'

export function DisciplineReviewPage() {

  const [filter, setFilter] = useState<ReviewFilter>({ page: 1, pageSize: 20 })
  const [selection, select] = useState<{ item: ReviewItem; decision: 'APPROVED' | 'DEFERRED' }>()
  const [form] = Form.useForm<{ reason: string }>()
  const client = useQueryClient()
  const queue = useQuery({ queryKey: ['discipline-review', filter], queryFn: () => searchReviews(filter) })
  const review = useMutation({
    mutationFn: (reason: string) => submitReview(selection!.item, selection!.decision, reason),
    onSuccess: () => { select(undefined); void client.invalidateQueries({ queryKey: ['semester-ledger'] }); void client.invalidateQueries({ queryKey: ['semester-ledger-detail'] }) },
    onSettled: () => client.invalidateQueries({ queryKey: ['discipline-review'] }),
  })
  function open(item: ReviewItem, decision: 'APPROVED' | 'DEFERRED') {
    form.resetFields(); review.reset(); select({ item, decision })
  }
  return <section className="attendance-page" aria-labelledby="discipline-review-title"><div className="attendance-page-heading"><Typography.Title level={2} id="discipline-review-title">待处分</Typography.Title></div>
    {queue.isError && <Alert type="error" title="待处理事项读取失败" action={<Button onClick={() => void queue.refetch()}>重试</Button>} />}
    <ProTable<ReviewItem, ReviewFilter> className="attendance-records-table discipline-review-table" cardBordered rowKey="id"
      rowClassName={row => row.canProcess ? 'discipline-row-actionable' : 'discipline-row-readonly'}
      headerTitle={<Space size={10}>待处分事项{queue.data && <Tag color="blue">共 {queue.data.total} 条</Tag>}<TableRowLegend label="待处分行底色表示处理权限" primary="可处理" secondary="只读" /></Space>}
      scroll={{ x: 'max-content' }} locale={{emptyText: queue.isError ? '待处分事项加载失败' : <Empty description="当前范围暂无待处分事项" />}} dataSource={queue.data?.items} loading={queue.isFetching}
      search={{ labelWidth: 'auto', defaultCollapsed: false }} options={false}
      onSubmit={values => setFilter({ ...values, page: 1, pageSize: filter.pageSize })}
      onReset={() => setFilter({ page: 1, pageSize: filter.pageSize })}
      pagination={{current: filter.page, pageSize: filter.pageSize, total: queue.data?.total, onChange: (page,pageSize) => setFilter({...filter,page:pageSize===filter.pageSize?page:1,pageSize})}}
      columns={[
        { title: '学号', dataIndex: 'student', hideInTable: true },
        { title: '学生', key: 'identity', search: false, render: (_, r) => <><strong>{r.studentName || '姓名未记录'}</strong><br/><Typography.Text type="secondary">{r.studentNumber}</Typography.Text></> },
        { title: '年级 / 专业', key: 'program', search: false, render: (_, r) => <>{r.gradeName || '—'}<br/>{r.majorName || '—'}</> },
        { title: '责任班级', dataIndex: 'className', search: false },
        { title: '触发累计项', dataIndex: 'groupName', search: false, render: (_, r) => r.groupName || '累计项未记录' },
        { title: '累计值', dataIndex: 'amount', search: false, render: (_, r) => [r.amount, r.unit].filter(v => v != null && v !== '').join(' ') },
        { title: '处分档位', dataIndex: 'tier', search: false },
        { title: '状态', dataIndex: 'status', valueType:'select', valueEnum: { PENDING:'待处理', DEFERRED:'已延迟' } },
        { title: '自动确认截止', search:false, render:(_,r)=>r.status==='DEFERRED'?'已延迟':new Date(r.dueAt).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai'}) },
        { title: '延迟原因', dataIndex:'reason', search:false },
        { title: '操作', search:false, render: (_,r)=>!r.canProcess ? <Typography.Text type="secondary">只读</Typography.Text> : <Space><Button type="link" onClick={()=>open(r,'APPROVED')}>确认处分</Button>{r.status==='PENDING' && <Button type="link" onClick={()=>open(r,'DEFERRED')}>延迟处理</Button>}</Space> },
      ]} />
    <Modal open={!!selection} title={selection?.decision==='DEFERRED'?'延迟处理':'确认处分'} okText="确定" cancelText="取消"
      confirmLoading={review.isPending} onCancel={()=>{if(!review.isPending)select(undefined)}} onOk={()=>form.submit()}>
      {selection && <p>{selection.item.studentName} {selection.item.studentNumber} · {selection.item.className}<br/>{selection.item.groupName || '累计项未记录'} · 累计 {selection.item.amount} {selection.item.unit} · {selection.item.tier}</p>}
      <Form form={form} onFinish={values=>review.mutate(values.reason?.trim() ?? '')}>
        {selection?.decision==='DEFERRED' && <Form.Item name="reason" label="延迟原因" rules={[{required:true,whitespace:true,message:'请填写延迟原因'}]}><Input.TextArea maxLength={500} /></Form.Item>}
      </Form>
      {review.isError && <Alert type="error" title={review.error.message} description="列表已重新读取，可关闭窗口核对最新状态。" />}
    </Modal>
  </section>
}
