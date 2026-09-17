import { describe, expect, it } from 'vitest'
import serverCatalog from '../../../../../server/internal/inspection/catalog.json'
import { getInspectionDefinitions, getInspectionDefinition } from './types'

describe('inspection field catalog', () => {
 it('matches the server catalog and preserves seven types / six tables', () => {
  const definitions=getInspectionDefinitions()
  expect(definitions).toEqual(serverCatalog)
  expect(definitions).toHaveLength(7)
  expect(new Set(definitions.map(d=>d.extensionTable)).size).toBe(6)
  expect(definitions.filter(d=>d.availableActions.length>0).map(d=>d.code)).toEqual(['ATTENDANCE_RATE'])
 })
 it('rejects unknown types with a stable validation code', () => {
  expect(()=>getInspectionDefinition('UNKNOWN')).toThrow('VALIDATION_FAILED')
 })
 it('keeps copies independent and morning activities combined', () => {
  const a=getInspectionDefinition('MORNING_ATTENDANCE')
  expect(a.fields.some(f=>f.key==='activityKind')).toBe(false)
  a.fields[0].label='changed'
  expect(getInspectionDefinition('MORNING_ATTENDANCE').fields[0].label).not.toBe('changed')
 })
})
