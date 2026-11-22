import { fireEvent, render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { OperationRangeEditor } from './OperationRangeEditor'

vi.mock('../model/useOperationRangeOptions', () => ({
  useOperationRangeOptions: () => ({
    data: { gradeId: [{ value: 'grade-2026', label: '2026级' }], majorId: [], classId: [] },
  }),
}))

it('旧阶段保留至管理员明确选择具体年级', async () => {
  const onChange = vi.fn()
  render(<OperationRangeEditor value={[{ gradeStage: 1 }]} onChange={onChange} />)
  expect(screen.getByText('请选择具体年级')).toBeVisible()
  expect(screen.queryByText('全部年级')).toBeNull()
  expect(onChange).not.toHaveBeenCalled()
  fireEvent.mouseDown(screen.getByRole('combobox', { name: '第1组年级' }))
  fireEvent.click(await screen.findByText('2026级'))
  expect(onChange.mock.lastCall?.[0]).toEqual([{ gradeId: 'grade-2026' }])
})
