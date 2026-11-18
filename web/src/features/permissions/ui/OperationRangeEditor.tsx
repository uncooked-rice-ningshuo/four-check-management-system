import { Alert, Button, Select, Typography } from 'antd'
import type { OperationRange } from '../../../entities/permission/model/permission'
import { useOperationRangeOptions } from '../model/useOperationRangeOptions'
import { classMatchesRange, expandRangeGroups, groupRanges, updateRangeConditions, type RangeGroup } from '../model/rangeGroups'
import './operation-range.css'


export function OperationRangeEditor({ value = [], onChange, disabled = false }: { value?: OperationRange[]; onChange?: (value: OperationRange[]) => void; disabled?: boolean }) {
  const choices = useOperationRangeOptions()
  const groups = groupRanges(value)
  const changeGroups = (next: RangeGroup[]) => onChange?.(expandRangeGroups(next))
  const patch = (index: number, change: Partial<RangeGroup>) => changeGroups(groups.map((item, at) => at === index ? updateRangeConditions(item, change, choices.data?.classId ?? []) : item))
  return <div className="operation-range-editor">
    {value.length > 0 && <Typography.Paragraph type="secondary">同组条件同时满足；多个班级任选其一</Typography.Paragraph>}
    {choices.isError && <Alert type="error" title="操作范围加载失败" action={<Button onClick={() => void choices.refetch()}>重试</Button>} />}
    {groups.map((range, index) => <fieldset key={index} className="operation-range-group"><legend><span>范围 {index + 1}</span><Button type="text" danger disabled={disabled || Boolean(range.gradeStage)} aria-label={`删除范围${index + 1}`} onClick={() => changeGroups(groups.filter((_, at) => at !== index))}>删除</Button></legend><div className="operation-range-fields">
      <label>年级<Select aria-label={`第${index + 1}组年级`} style={{ width: '100%' }} placeholder="全部年级" allowClear={!range.gradeStage} showSearch optionFilterProp="label" value={range.gradeStage ? `待调整：大${['一', '二', '三', '四'][range.gradeStage - 1]}` : range.gradeId} disabled={disabled || choices.isPending || choices.isError} options={choices.data?.gradeId} onChange={gradeId => patch(index, { gradeStage: undefined, gradeId })} /></label>
      <label>专业<Select aria-label={`第${index + 1}组专业`} style={{ width: '100%' }} placeholder="全部专业" allowClear showSearch optionFilterProp="label" value={range.majorId} disabled={disabled || choices.isPending || choices.isError} options={choices.data?.majorId} onChange={majorId => patch(index, { majorId })} /></label>
      <label>班级（可多选）<Select mode="multiple" aria-label={`第${index + 1}组班级`} style={{ width: '100%' }} placeholder="全部班级" allowClear showSearch optionFilterProp="label" value={range.classIds} disabled={disabled || choices.isPending || choices.isError} options={choices.data?.classId.filter(option => classMatchesRange(option, range))} notFoundContent="没有符合年级和专业条件的班级" onChange={classIds => patch(index, { classIds })} /></label>
      </div>
    </fieldset>)}
    <Typography.Paragraph className="operation-range-summary" type="secondary">可操作：{groups.length ? groups.map(range => [range.gradeStage ? `本学年${['大一', '大二', '大三', '大四'][range.gradeStage - 1]}` : '', ...(['gradeId', 'majorId'] as const).map(key => range[key] ? choices.data?.[key].find(item => item.value === range[key])?.label ?? '加载中' : ''), range.classIds.length ? `${range.classIds.length} 个已选班级` : ''].filter(Boolean).join('、') || '请选择条件').join('；或 ') : '不限'}</Typography.Paragraph>
    <Button type="dashed" block disabled={disabled || choices.isPending || choices.isError} onClick={() => changeGroups([...groups, { classIds: [] }])}>添加一组范围</Button>
  </div>
}
