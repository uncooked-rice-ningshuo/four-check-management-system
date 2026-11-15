import { useQuery } from '@tanstack/react-query'
import { Alert, Button, Form, Select, Typography } from 'antd'
import { getMasterData, previewReportGrades } from '../../../shared/api/adminApi'

async function loadGrades() {
  const first = await getMasterData('grades', { page: 1, pageSize: 100 })
  const items = [...first.items]
  for (let page = 2; items.length < first.total; page += 1) {
    const next = await getMasterData('grades', { page, pageSize: 100 })
    if (!next.items.length) throw new Error('年级选项加载不完整，请重试')
    items.push(...next.items)
  }
  return items
}

export function AcademicYearStageFields() {
  const selected = Form.useWatch('stageGrades') as Record<string, string | undefined> | undefined
  const gradeIds = [...new Set(Object.values(selected ?? {}).filter((id): id is string => Boolean(id)))].sort()
  const preview = useQuery({ queryKey: ['academic', 'report-grade-preview', gradeIds], queryFn: () => previewReportGrades(gradeIds), enabled: gradeIds.length > 0 })
  const grades = useQuery({ queryKey: ['master-data', 'all-grades'], queryFn: loadGrades })
  return <>
    <Typography.Paragraph type="secondary">本学年各学期共用此映射，用于年级部范围匹配；已配置的年级共同组成通报覆盖年级。</Typography.Paragraph>
    {grades.isError && <Alert type="error" title="年级选项加载失败" action={<Button onClick={() => void grades.refetch()}>重试</Button>} />}
    {([1, 2, 3, 4] as const).map((stage, index) => <Form.Item key={stage} name={['stageGrades', String(stage)]} label={['大一', '大二', '大三', '大四'][index]} validateTrigger="onBlur" dependencies={[['stageGrades']]} rules={[({ getFieldValue }) => ({ validator(_, value: string | undefined) {
      const values = Object.values(getFieldValue('stageGrades') ?? {})
      return value && values.filter(item => item === value).length > 1 ? Promise.reject(new Error('同一学年的各阶段不能选择相同年级')) : Promise.resolve()
    } })]}>
      <Select allowClear showSearch optionFilterProp="label" placeholder="选择对应入学年级" loading={grades.isLoading} disabled={grades.isLoading || grades.isError} options={(grades.data ?? []).map(grade => ({ value: grade.id, label: grade.name, disabled: !grade.enabled }))} />
    </Form.Item>)}
    {preview.isError && <Alert type="error" title="通报名册预览加载失败" action={<Button onClick={() => void preview.refetch()}>重试</Button>} />}
    {preview.data && gradeIds.length > 0 && <Typography.Paragraph>通报覆盖：{preview.data.grades.map(item => item.gradeName).join('、')}；启用班级 {preview.data.classCount} 个，在籍学生 {preview.data.studentCount} 人，未分入启用班级 {preview.data.unassignedCount} 人</Typography.Paragraph>}
  </>
}
