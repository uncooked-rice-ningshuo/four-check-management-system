import { expect, it } from 'vitest'
import { canDeleteAttendance, canMutateAttendance } from './canMutateAttendance'
import type { AttendanceRecord } from '../../../entities/inspection/model/inspection'
import type { EffectivePermission } from '../../../entities/permission/model/permission'
const record: AttendanceRecord = { id: 'r', semesterId: 's', weekId: 'w', inspectionTypeId: 't', departmentId: 'd', createdBy: 'owner', classId: 'c', grade: '2026级', status: 'DRAFT', source: 'MANUAL', studentNo: 'S1', studentName: '虚构同学', className: '测试班', courseId: 'course', courseName: '课程', occurredAt: '2026-09-06T00:00:00Z', absenceHours: 1, period: '1', remark: '', rowVersion: 1 }
const grant = (action: string, effect: 'ALLOW' | 'DENY' = 'ALLOW'): EffectivePermission => ({ action, effect, scope: {}, sources: [] })
it('separates own entry permission from others management permission', () => {
 expect(canMutateAttendance(record, { id: 'owner', isSuperAdmin: false }, [grant('INSPECTION_ENTER')])).toBe(true)
 expect(canMutateAttendance(record, { id: 'other', isSuperAdmin: false }, [grant('INSPECTION_ENTER')])).toBe(false)
 expect(canMutateAttendance(record, { id: 'other', isSuperAdmin: false }, [grant('INSPECTION_MANAGE')])).toBe(true)
 expect(canMutateAttendance(record, { id: 'other', isSuperAdmin: false }, [grant('INSPECTION_MANAGE'), grant('INSPECTION_MANAGE', 'DENY')])).toBe(false)
})

it('separates draft deletion from returned-record editing and allows scoped managers to edit administrator records', () => {
 expect(canDeleteAttendance({ ...record, canDelete: true }, { id: 'owner', isSuperAdmin: false }, [grant('INSPECTION_ENTER')])).toBe(true)
 expect(canDeleteAttendance({ ...record, status: 'RETURNED', canDelete: false }, { id: 'owner', isSuperAdmin: false }, [grant('INSPECTION_ENTER')])).toBe(false)
 expect(canMutateAttendance({ ...record, attributionMode: 'ADMINISTRATOR' }, { id: 'other', isSuperAdmin: false }, [grant('INSPECTION_MANAGE')])).toBe(true)
 expect(canMutateAttendance({ ...record, attributionMode: 'ADMINISTRATOR' }, { id: 'other', isSuperAdmin: false }, [grant('INSPECTION_ENTER')])).toBe(false)
 expect(canDeleteAttendance({ ...record, attributionMode: 'ADMINISTRATOR', canDelete: true }, { id: 'other', isSuperAdmin: false }, [grant('INSPECTION_MANAGE')])).toBe(true)
 expect(canMutateAttendance({ ...record, attributionMode: 'ADMINISTRATOR' }, { id: 'owner', isSuperAdmin: false }, [grant('INSPECTION_ENTER')])).toBe(true)
})
it('allows a scoped member to correct a returned record', () => {
 expect(canMutateAttendance({ ...record, status: 'RETURNED' }, { id: 'owner', isSuperAdmin: false }, [grant('INSPECTION_ENTER')])).toBe(true)
})
it.each(['PENDING_REVIEW', 'CONFIRMED', 'LOCKED'])('hides %s actions even for super admins', status => {
 expect(canMutateAttendance({ ...record, status }, { id: 'root', isSuperAdmin: true }, [])).toBe(false)
})
