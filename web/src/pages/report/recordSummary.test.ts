import { describe, expect, it } from 'vitest'
import type { ReportingRecord } from '../../entities/reporting/model/reporting'
import { absenceSummary } from './recordSummary'

describe('到课率缺课摘要', () => {
  it('以节次而不是小时展示缺课数量', () => {
    const record: ReportingRecord = { id: 'r', typeCode: 'ATTENDANCE_RATE', studentNumber: '', studentName: '', className: '', grade: '', currentDormitoryName: '', currentCourseName: '', currentClassroomName: '', details: { absenceHours: 2 } }
    expect(absenceSummary(record)).toBe('缺课 2 节次')
  })
})
