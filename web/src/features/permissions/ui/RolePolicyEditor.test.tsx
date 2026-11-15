import { StrictMode, useState } from 'react'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { RolePolicy } from '../../../entities/organization/model/organization'
import { RolePolicyEditor } from './RolePolicyEditor'

function Example() {
  const [policies, setPolicies] = useState<RolePolicy[]>([{ roleCode: 'MEMBER', validityYears: 1, canManageMorningSchedule: false, operationRanges: [] }])
  return <RolePolicyEditor policies={policies} roleCode="MEMBER" moduleKey="test" onChange={setPolicies} />
}
describe('module validity confirmation', () => {
  it('keeps edits local until confirmed and discards cancelled edits on reopen', () => {
    render(<StrictMode><Example /></StrictMode>)
    fireEvent.click(screen.getByRole('button', { name: '委员有效期：授予后 1 年' }))
    let popup = screen.getByRole('tooltip')
    fireEvent.change(within(popup).getByRole('spinbutton'), { target: { value: '3' } })
    expect(screen.getByRole('button', { name: '委员有效期：授予后 1 年' })).toBeVisible()
    fireEvent.click(within(popup).getByRole('button', { name: /取\s*消/ }))
    fireEvent.click(screen.getByRole('button', { name: '委员有效期：授予后 1 年' }))
    popup = screen.getByRole('tooltip')
    expect(within(popup).getByRole('spinbutton')).toHaveValue('1')
    fireEvent.change(within(popup).getByRole('spinbutton'), { target: { value: '2' } })
    fireEvent.click(within(popup).getByRole('button', { name: /确\s*定/ }))
    expect(screen.getByRole('button', { name: '委员有效期：授予后 2 年' })).toBeVisible()
  })
})
