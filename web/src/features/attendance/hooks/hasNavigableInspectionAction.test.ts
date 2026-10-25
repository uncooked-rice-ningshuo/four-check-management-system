import { expect, it } from 'vitest'
import { hasNavigableInspectionAction } from './hasNavigableInspectionAction'
import type { EffectivePermission } from '../../../entities/permission/model/permission'

it('removes fully denied grants but retains partially available scopes', () => {
  const allow: EffectivePermission = { action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: {}, sources: [] }
  const deny: EffectivePermission = { ...allow, effect: 'DENY' }
  expect(hasNavigableInspectionAction([allow, deny], allow.action)).toBe(false)
  expect(hasNavigableInspectionAction([allow, { ...deny, scope: { weekId: 'one' } }], allow.action)).toBe(true)
  expect(hasNavigableInspectionAction([{ ...allow, scope: { weekId: 'one' } }, { ...deny, scope: { weekId: 'one' } }], allow.action)).toBe(false)
  expect(hasNavigableInspectionAction([deny], allow.action)).toBe(false)
})

it('intersects navigation grants and denies with the requested inspection type', () => {
 const allow: EffectivePermission={action:'INSPECTION_VIEW',effect:'ALLOW',scope:{inspectionTypeId:'attendance'},sources:[]}
 expect(hasNavigableInspectionAction([allow],allow.action,'unattended')).toBe(false)
 expect(hasNavigableInspectionAction([{...allow,scope:{}},{...allow,effect:'DENY',scope:{inspectionTypeId:'unattended'}}],allow.action,'unattended')).toBe(false)
 expect(hasNavigableInspectionAction([{...allow,scope:{}},{...allow,effect:'DENY'}],allow.action,'unattended')).toBe(true)
})

it('uses inspection type for navigation and does not add department scope', () => {
 const allow:EffectivePermission={action:'INSPECTION_ENTER',effect:'ALLOW',scope:{},sources:[]}
 const deny:EffectivePermission={...allow,effect:'DENY',scope:{inspectionTypeId:'attendance'}}
 expect(hasNavigableInspectionAction([allow,deny],allow.action,'attendance')).toBe(false)
 expect(hasNavigableInspectionAction([allow,deny],allow.action,'sports')).toBe(true)
})
