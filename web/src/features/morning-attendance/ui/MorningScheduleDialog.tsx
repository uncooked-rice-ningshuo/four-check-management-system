import { Alert, Button, Checkbox, Drawer, Form, Space, Spin, Typography, message } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { ApiError } from '../../../shared/api/httpClient'
import { morningScheduleApi } from '../../../shared/api/morningScheduleApi'

const weekdayLabels = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']

function scheduleDates(startDate: string, endDate: string) {
  const firstDay = Date.parse(`${startDate}T00:00:00Z`)
  const lastDay = Date.parse(`${endDate}T00:00:00Z`)
  const dayCount = Math.floor((lastDay - firstDay) / 86_400_000) + 1
  return Array.from({ length: Math.max(0, dayCount) }, (_, index) => {
    const date = new Date(firstDay + index * 86_400_000)
    return { value: date.toISOString().slice(0, 10), weekday: weekdayLabels[date.getUTCDay()] }
  })
}

export function MorningScheduleDialog({ weekId, onClose }: { weekId: string; onClose: () => void }) {
  const queryClient = useQueryClient()
  const schedule = useQuery({ queryKey: ['morning-schedule', weekId], queryFn: () => morningScheduleApi.get(weekId), enabled: !!weekId })
  const [selectedOverride, setSelectedOverride] = useState<string[]>()
  const [error, setError] = useState('')
  const selected = selectedOverride ?? schedule.data?.plannedDates ?? []
  const save = useMutation({
    mutationFn: () => morningScheduleApi.save(weekId, schedule.data?.version ?? 0, selected),
    onSuccess: async value => {
      setSelectedOverride(undefined)
      await queryClient.setQueryData(['morning-schedule', weekId], value)
      await queryClient.invalidateQueries({ queryKey: ['morning-attendances'] })
      message.success('早签计划已保存')
      onClose()
    },
    onError: failure => setError(failure instanceof ApiError && failure.status < 500 ? failure.message : '保存早签计划失败，请刷新后重试。'),
  })
  const dates = schedule.data ? scheduleDates(schedule.data.startDate, schedule.data.endDate) : []
  const setDate = (date: string, checked: boolean) => setSelectedOverride(current => {
    const currentDates = current ?? schedule.data?.plannedDates ?? []
    return checked ? [...new Set([...currentDates, date])].sort() : currentDates.filter(value => value !== date)
  })

  return <Drawer open title="早签计划" size={480} onClose={save.isPending ? undefined : onClose} closable={!save.isPending} maskClosable={!save.isPending}
    extra={<Space><Button onClick={onClose} disabled={save.isPending}>取消</Button><Button type="primary" onClick={() => { setError(''); save.mutate() }} loading={save.isPending} disabled={!schedule.data?.canEdit || schedule.isError}>保存</Button></Space>}>
    {schedule.isLoading && <Spin />}
    {schedule.isError && <Alert type="error" showIcon message="早签计划加载失败" action={<Button onClick={() => void schedule.refetch()}>重试</Button>} />}
    {error && <Alert type="error" showIcon message={error} style={{ marginBottom: 16 }} />}
    {schedule.data && <>
      <Typography.Paragraph type="secondary">
        {schedule.data.startDate} 至 {schedule.data.endDate}。未单独配置时，默认安排周一至周五；此计划用于手工录入校验，Excel 导入始终按默认工作日校验。
      </Typography.Paragraph>
      {!schedule.data.canEdit && <Typography.Paragraph type="secondary">当前账号仅可查看此计划。</Typography.Paragraph>}
      {schedule.data.isDefault && <Typography.Paragraph type="secondary">当前使用默认工作日计划；保存后将单独保存此周配置。</Typography.Paragraph>}
      <Form layout="vertical"><Form.Item label="早签安排日期">
        <Space orientation="vertical" size={10}>
          {dates.map(date => {
            return <Checkbox key={date.value} checked={selected.includes(date.value)} disabled={!schedule.data?.canEdit} onChange={event => setDate(date.value, event.target.checked)}>
              {date.value}（{date.weekday}）
            </Checkbox>
          })}
        </Space>
      </Form.Item></Form>
    </>}
  </Drawer>
}
