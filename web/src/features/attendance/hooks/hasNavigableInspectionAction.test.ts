import { expect, it } from 'vitest'
import { hasNavigableInspectionAction } from './hasNavigableInspectionAction'
import type { EffectivePermission } from '../../../entities/permission/model/permission'

it('removes fully denied grants but retains partially available scopes', () => {
  const allow: EffectivePermission = { action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: {}, sources: [] }
  const deny: EffectivePermission = { ...allow, effect: 'DENY' }
  expect(hasNavigableInspectionAction([allow, deny], allow.action)).toBe(false)
  expect(hasNavigableInspectionAction([allow, { ...deny, scope: { classId: 'one' } }], allow.action)).toBe(true)
  expect(hasNavigableInspectionAction([{ ...allow, scope: { classId: 'one' } }, { ...deny, scope: { classId: 'one' } }], allow.action)).toBe(false)
  expect(hasNavigableInspectionAction([deny], allow.action)).toBe(false)
})
