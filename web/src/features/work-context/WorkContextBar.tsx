import { ProFormDateTimePicker } from '@ant-design/pro-components'
import { Alert, Button, Form, Input, Select, Space } from 'antd'
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getAcademicCalendar } from '../../shared/api/adminApi'
import { useSetWorkContext, useWorkContext } from './useWorkContext'

export function WorkContextBar({ canManage = false }: { canManage?: boolean }) {
  const query = useWorkContext()
  const save = useSetWorkContext()
  const calendar = useQuery({ queryKey: ['academic', 'calendar', 'override'], queryFn: getAcademicCalendar, enabled: canManage })
  const [version, setVersion] = useState<number | null>(null)
  const [weekId, setWeekId] = useState('')
  const [form] = Form.useForm<{ expiry?: string }>()
  const expiry = Form.useWatch('expiry', form)
  const [reason, setReason] = useState('')
  if (query.isPending) return <div role="status">正在读取工作周次</div>
  if (query.isError) return <Alert type="warning" title="读取工作周次失败" action={<Button aria-label="重试" onClick={() => void query.refetch()}>重试</Button>} />
  const context = query.data
  const update = async (clear: boolean) => {
    if (version === null) return
    try {
      await save.mutateAsync({ version, weekId: clear ? '' : weekId, expiresAt: clear ? null : new Date(`${expiry?.replace(' ', 'T')}:00+08:00`).toISOString(), reason })
      setVersion(null)
    } catch { /* Mutation state retains the error and original version for explicit retry. */ }
  }
  const weeks = (calendar.data?.weeks ?? []).filter(w => w.enabled && calendar.data?.semesters.some(s => s.id === w.semesterId && s.enabled && calendar.data?.academicYears.some(y => y.id === s.academicYearId && y.enabled)))
  return <section aria-label="当前工作上下文" style={{ padding: '12px 24px' }}>
    <Space wrap>
      <strong>{context.week ? `当前工作周：第 ${context.week.weekNo} 周（${context.week.startDate.slice(0, 10)} 至 ${context.week.endDate.slice(0, 10)}）` : '当前无有效教学周，请选择工作范围'}</strong>
      {context.source === 'OVERRIDE' && <span>临时覆盖，至 {context.expiresAt ? new Date(context.expiresAt).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' }) : ''}（北京时间）</span>}
      {canManage && version === null && <Button onClick={() => { setVersion(context.version); setWeekId(context.week?.id ?? ''); form.resetFields(); setReason(''); save.reset() }}>设置临时工作周</Button>}
    </Space>
    {canManage && version !== null && <div>
      <p>此设置影响全体用户的默认工作周，到期自动恢复校历，不修改已有记录或已打开的录入内容。</p>
      {calendar.isError && <Alert type="error" title="读取校历失败" />}
      <Form form={form}><Space wrap>
        <label>目标周次 <Select aria-label="目标周次" value={weekId || undefined} onChange={setWeekId} placeholder="请选择" loading={calendar.isFetching} style={{ minWidth: 180 }} options={weeks.map(w => ({ value: w.id, label: `${calendar.data?.semesters.find(s => s.id === w.semesterId)?.name} 第 ${w.weekNo} 周` }))} /></label>
        <ProFormDateTimePicker name="expiry" label="到期时间（北京时间）" getValueFromEvent={(_date: unknown,dateString: string)=>dateString||undefined} fieldProps={{format:'YYYY-MM-DD HH:mm',style:{minWidth:200}}}/>
        <Input aria-label="覆盖原因" placeholder="覆盖或恢复原因" maxLength={255} value={reason} onChange={e => setReason(e.target.value)} />
        <Button loading={save.isPending} disabled={!weekId || !expiry || !reason.trim()} onClick={() => void update(false)}>确认全局覆盖</Button>
        <Button disabled={!reason.trim() || save.isPending} onClick={() => void update(true)}>恢复校历</Button>
        <Button disabled={save.isPending} onClick={() => setVersion(null)}>取消</Button>
      </Space></Form>
      {save.isError && <Alert type="error" title="更新失败，请核对周次、到期时间；如版本冲突，请取消后重新打开" />}
    </div>}
  </section>
}
