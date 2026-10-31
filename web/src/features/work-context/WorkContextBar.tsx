import { ProFormDateTimePicker } from '@ant-design/pro-components'
import { CalendarOutlined, SettingOutlined } from '@ant-design/icons'
import { Alert, Button, Form, Input, Modal, Select, Space, Tag, Typography } from 'antd'
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
  if (query.isPending) return <div className="work-context-header" role="status">正在读取工作周次</div>
  if (query.isError) return <div className="work-context-header"><Alert type="warning" title="读取工作周次失败" action={<Button aria-label="重试" onClick={() => void query.refetch()}>重试</Button>} /></div>
  const context = query.data
  const update = async (clear: boolean) => {
    if (version === null) return
    try {
      await save.mutateAsync({ version, weekId: clear ? '' : weekId, expiresAt: clear ? null : new Date(`${expiry?.replace(' ', 'T')}:00+08:00`).toISOString(), reason })
      setVersion(null)
    } catch { /* Mutation state retains the error and original version for explicit retry. */ }
  }
  const weeks = (calendar.data?.weeks ?? []).filter(w => w.enabled && calendar.data?.semesters.some(s => s.id === w.semesterId && s.enabled && calendar.data?.academicYears.some(y => y.id === s.academicYearId && y.enabled)))
  return <div className="work-context-header" role="group" aria-label="当前工作上下文">
    <CalendarOutlined className="work-context-icon" aria-hidden="true" />
    <div className="work-context-copy">
      <Typography.Text className="work-context-label">当前工作周</Typography.Text>
      {context.week ? <>
        <Typography.Text className="work-context-week" title={`第 ${context.week.weekNo} 周`}>第 {context.week.weekNo} 周</Typography.Text>
        <Typography.Text className="work-context-dates" title={`${context.week.startDate.slice(0, 10)} 至 ${context.week.endDate.slice(0, 10)}`}>
          {context.week.startDate.slice(0, 10)} 至 {context.week.endDate.slice(0, 10)}
        </Typography.Text>
      </> : <Typography.Text className="work-context-empty" title="当前无有效教学周，请选择工作范围">当前无有效教学周，请选择工作范围</Typography.Text>}
    </div>
    {context.source === 'OVERRIDE' && <Tag color="gold" className="work-context-override" title={`临时覆盖，至 ${context.expiresAt ? new Date(context.expiresAt).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' }) : '未设置到期时间'}（北京时间）`}>临时覆盖</Tag>}
    {canManage && <Button
      className="work-context-manage"
      icon={<SettingOutlined aria-hidden="true" />}
      aria-label="设置临时工作周"
      onClick={() => { setVersion(context.version); setWeekId(context.week?.id ?? ''); form.resetFields(); setReason(''); save.reset() }}
    ><span>设置临时工作周</span></Button>}
    <Modal
      title="设置临时工作周"
      open={version !== null}
      onCancel={() => { if (!save.isPending) setVersion(null) }}
      footer={null}
      destroyOnHidden={false}
    >
      <Space orientation="vertical" size="middle" style={{ display: 'flex' }}>
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          此设置影响全体用户的默认工作周，到期自动恢复校历，不修改已有记录或已打开的录入内容。
        </Typography.Paragraph>
        {calendar.isError && <Alert type="error" title="读取校历失败" />}
        <Form form={form} layout="vertical">
          <Form.Item label="目标周次" required>
            <Select aria-label="目标周次" value={weekId || undefined} onChange={setWeekId} placeholder="请选择" loading={calendar.isFetching} options={weeks.map(w => ({ value: w.id, label: `${calendar.data?.semesters.find(s => s.id === w.semesterId)?.name} 第 ${w.weekNo} 周` }))} />
          </Form.Item>
          <ProFormDateTimePicker name="expiry" label="到期时间（北京时间）" getValueFromEvent={(_date: unknown, dateString: string) => dateString || undefined} fieldProps={{ format: 'YYYY-MM-DD HH:mm', style: { width: '100%' } }} />
          <Form.Item label="覆盖原因" required>
            <Input aria-label="覆盖原因" placeholder="填写覆盖或恢复原因" maxLength={255} value={reason} onChange={event => setReason(event.target.value)} />
          </Form.Item>
        </Form>
        {save.isError && <Alert type="error" showIcon title="更新失败，请核对周次、到期时间；如版本冲突，请关闭后重新打开" />}
        <Space wrap>
          <Button type="primary" loading={save.isPending} disabled={!weekId || !expiry || !reason.trim()} onClick={() => void update(false)}>确认全局覆盖</Button>
          <Button disabled={!reason.trim() || save.isPending} onClick={() => void update(true)}>恢复校历</Button>
          <Button disabled={save.isPending} onClick={() => setVersion(null)}>取消</Button>
        </Space>
      </Space>
    </Modal>
  </div>
}
