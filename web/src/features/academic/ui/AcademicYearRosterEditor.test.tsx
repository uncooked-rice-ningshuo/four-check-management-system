import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { expect, it } from 'vitest'
import { AcademicYearRosterEditor } from './AcademicYearRosterEditor'

it('在专业行展示当前年级启用班级总数，包含未勾选班级', () => {
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } })
  client.setQueryData(['academic', 'roster-options'], [
    ['26', '27'].map(id => ({ id, name: id + '级', enabled: true })),
    [{ id: 'cs', name: '计科', enabled: true }, { id: 'se', name: '软件', enabled: true }],
    [['a', '26', true], ['b', '26', true], ['c', '27', true], ['d', '27', false]].map(([id, gradeId, enabled]) =>
      ({ id, gradeId, enabled, name: String(id), majorId: 'cs' })),
  ])
  render(<QueryClientProvider client={client}><AcademicYearRosterEditor step={0} startYear={2026} years={[]}
    value={{ majorIds: ['cs'], grades: ['26', '27'].map(gradeId => ({ gradeId, majors: [{ majorId: 'cs', classIds: [] }] })) }} /></QueryClientProvider>)
  const count = (major: string, total: number) => expect(within(screen.getByRole('treeitem', { name: new RegExp(major) })).getByText(total + ' 个班')).toBeVisible()
  count('计科', 2)
  count('软件', 0)
  fireEvent.click(screen.getByRole('button', { name: /27级/ }))
  count('计科', 1)
  count('软件', 0)
})
