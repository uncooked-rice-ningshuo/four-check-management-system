import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Modal, Select, Typography, message } from 'antd'
import type { Semester } from '../../../entities/academic/model/academic'
import { getMasterData, getSemesterReportGrades, replaceSemesterReportGrades } from '../../../shared/api/adminApi'
import type { MasterDataRecord } from '../../../entities/masterdata/model/masterData'

async function allGrades(): Promise<MasterDataRecord[]> {
  const first = await getMasterData('grades', { page: 1, pageSize: 100 })
  const items = [...first.items]
  for (let page = 2; items.length < first.total; page += 1) {
    const next = await getMasterData('grades', { page, pageSize: 100 })
    if (!next.items.length) throw new Error('年级选项加载不完整，请重试')
    items.push(...next.items)
  }
  return items
}

export function SemesterReportGradesDialog({ semester, onClose }: { semester: Semester; onClose: () => void }) {
  const client = useQueryClient()
  const [messageApi, messageContextHolder] = message.useMessage()
  const [selectedGradeIds, setSelectedGradeIds] = useState<string[] | null>(null)
  const scope = useQuery({ queryKey: ['semester-report-grades', semester.id], queryFn: () => getSemesterReportGrades(semester.id) })
  const grades = useQuery({ queryKey: ['master-data', 'all-grades'], queryFn: allGrades })
  const gradeIds = selectedGradeIds ?? scope.data?.gradeIds ?? []
  const save = useMutation({
    mutationFn: () => replaceSemesterReportGrades({ semesterId: semester.id, gradeIds, version: scope.data?.version ?? 0 }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['semester-report-grades', semester.id] })
      void messageApi.success('适用年级已保存')
      onClose()
    },
    onError: error => { void messageApi.error(error.message) },
  })
  const loading = scope.isLoading || grades.isLoading
  const loadFailed = scope.isError || grades.isError
  return <Modal open title={`${semester.name} · 通报适用年级`} onCancel={onClose} onOk={() => save.mutate()} okText="保存" okButtonProps={{ disabled: loading || loadFailed || !gradeIds.length }} confirmLoading={save.isPending} destroyOnHidden>
    {messageContextHolder}
    <Typography.Paragraph>该学期的所有周次继承此范围。新通报版本使用保存时的适用年级和当前在籍名册，历史版本保持原快照。</Typography.Paragraph>
    {loadFailed && <Alert type="error" showIcon title="适用年级加载失败" description="请重试加载后再保存。" action={<a onClick={() => { void scope.refetch(); void grades.refetch() }}>重试</a>} />}
    {!loadFailed && <Select mode="multiple" style={{ width: '100%' }} placeholder="选择适用年级" value={gradeIds} onChange={setSelectedGradeIds} loading={loading} disabled={loading} options={(grades.data ?? []).map(item => ({ value: item.id, label: item.name, disabled: !item.enabled }))} optionFilterProp="label" />}
    {!loading && !loadFailed && <Typography.Text type="secondary">已选择 {gradeIds.length} 个年级；不限定为四个。未配置时不能生成新的全范围通报。</Typography.Text>}
  </Modal>
}
