import { describe, expect, it } from 'vitest'
import serverCatalog from '../../../../../server/internal/inspection/catalog.json'
import { getInspectionDefinitions, getInspectionDefinition } from './types'

describe('inspection field catalog', () => {
 it('matches the server catalog and preserves seven types / six tables', () => {
  const definitions=getInspectionDefinitions()
  expect(definitions).toEqual(serverCatalog)
  expect(definitions).toHaveLength(7)
  expect(new Set(definitions.map(d=>d.extensionTable)).size).toBe(6)
  expect(definitions.filter(d=>d.availableActions.length>0).map(d=>d.code)).toEqual(['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'CLASSROOM_HYGIENE', 'MORNING_ATTENDANCE', 'DORMITORY_HYGIENE', 'LATE_RETURN', 'PUBLIC_DORMITORY_HYGIENE'])
 })
 it('exposes verified unattended manual and import capabilities', () => {
  expect(getInspectionDefinition('UNATTENDED_CLASSROOM').availableActions).toEqual(['CREATE','LIST','EDIT','SOFT_DELETE','BATCH_DELETE','IMPORT'])
 })
 it('exposes verified late-return import', () => {
  expect(getInspectionDefinition('LATE_RETURN').availableActions).toEqual(['CREATE','LIST','EDIT','SOFT_DELETE','BATCH_DELETE','IMPORT'])
 })
 it('exposes morning import without a subtype', () => {
  const morning=getInspectionDefinition('MORNING_ATTENDANCE')
  expect(morning.availableActions).toEqual(['CREATE','LIST','EDIT','SOFT_DELETE','BATCH_DELETE','IMPORT'])
  expect(morning.templateStatus).toBe('ACTIVE')
  expect(morning.fields.some(field=>field.key==='activityKind')).toBe(false)
 })
 it('exposes verified dormitory hygiene import',()=>{
  expect(getInspectionDefinition('CLASSROOM_HYGIENE').availableActions).toEqual(['CREATE','LIST','EDIT','SOFT_DELETE','BATCH_DELETE','IMPORT'])
  expect(getInspectionDefinition('DORMITORY_HYGIENE').availableActions).toEqual(['CREATE','LIST','EDIT','SOFT_DELETE','BATCH_DELETE','IMPORT'])
 })
 it('keeps the two other verified hygiene pages manual-only',()=>{
  for(const code of ['PUBLIC_DORMITORY_HYGIENE'] as const){
   expect(getInspectionDefinition(code).availableActions).toEqual(['CREATE','LIST','EDIT','SOFT_DELETE','BATCH_DELETE'])
   expect(getInspectionDefinition(code).templateStatus).toBe('PLANNED')
  }
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
