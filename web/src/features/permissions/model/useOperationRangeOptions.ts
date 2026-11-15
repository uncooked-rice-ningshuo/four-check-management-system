import { useQuery } from '@tanstack/react-query'
import { getMasterData } from '../../../shared/api/adminApi'

async function options(kind: 'grades' | 'majors' | 'classes' | 'faculties') {
  const first = await getMasterData(kind, { page: 1, pageSize: 100 })
  const items = [...first.items]
  for (let page = 2; items.length < first.total; page += 1) {
    const next = await getMasterData(kind, { page, pageSize: 100 })
    if (!next.items.length) throw new Error('操作范围选项加载不完整，请重试')
    items.push(...next.items)
  }
  return items
}


export function useOperationRangeOptions() {
  return useQuery({ queryKey: ['permission-range-options'], queryFn: async () => {
    const [grades, majors, classes, faculties] = await Promise.all([options('grades'), options('majors'), options('classes'), options('faculties')])
    const name = (items: typeof grades, id?: string) => items.find(item => item.id === id)?.name
    return {
      gradeId: grades.map(item => ({ value: item.id, label: item.name, disabled: !item.enabled })),
      majorId: majors.map(item => ({ value: item.id, label: [name(faculties, item.facultyId), item.name].filter(Boolean).join('／'), disabled: !item.enabled })),
      classId: classes.map(item => ({ value: item.id, gradeId: item.gradeId, majorId: item.majorId, label: [name(grades, item.gradeId) ?? item.grade, name(majors, item.majorId), item.name].filter(Boolean).join('／'), disabled: !item.enabled })),
    }
  } })
}
