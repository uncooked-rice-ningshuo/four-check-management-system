import { fireEvent, render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import type { ReportVersion } from '../../../entities/reporting/model/reporting'
import { ReportVersionIndicators } from './ReportVersionIndicators'

function versionFixture(): ReportVersion {
  return {
    id: 'v1', versionNo: 1, createdBy: 'actor', createdAt: '2026-01-08',
    content: {
      period: { academicYearId: 'year', academicYearName: '学年', semesterId: 'semester', semesterName: '学期', weekId: 'week', weekNo: 1, startDate: '2026-01-01', endDate: '2026-01-07' },
      scopeLimited: false, statisticsRuleVersion: 'WEEKLY_INDICATORS_V2', confirmedRecordCount: 0,
      rows: [], incompleteCells: [{ departmentId: 'department', departmentName: '学习部', collegeLevel: false, inspectionTypeId: 'type', inspectionType: '到课率' }], corrections: [], generatedAt: '2026-01-08',
      indicators: {
        inputs: { weekMultiplier: 1, morningPlannedDates: [], classes: [] }, includedTypeCodes: ['ATTENDANCE_RATE'],
        sourceStates: { ATTENDANCE_RATE: 'SOURCE_INCOMPLETE' },
        totals: { expectedStudentCount: 2, attendanceRate: { expectedStudentCount: 2, attendedStudentCount: 2, violationStudentCount: 0, rate: 100 } },
        classes: [
          { classId: 'a', grade: '2026级', className: '一班', expectedStudentCount: 2, sourceStates: { ATTENDANCE_RATE: 'SOURCE_INCOMPLETE' }, attendanceRate: { expectedStudentCount: 2, attendedStudentCount: 2, violationStudentCount: 0, rate: 100 } },
          { classId: 'b', grade: '2026级', className: '二班', expectedStudentCount: 0, sourceStates: { ATTENDANCE_RATE: 'SOURCE_INCOMPLETE' }, attendanceRate: { expectedStudentCount: 0, attendedStudentCount: 0, violationStudentCount: 0, rate: null } },
        ],
      },
    },
  }
}

it('keeps all scoped classes visible without showing an incomplete source as a valid rate', () => {
  const version = versionFixture()
  render(<ReportVersionIndicators version={version} />)
  expect(screen.getByText('一班')).toBeVisible()
  expect(screen.getByText('二班')).toBeVisible()
  expect(screen.getAllByText('来源未完成')).toHaveLength(2)
  expect(screen.queryByText('100.00%')).not.toBeInTheDocument()
})

it('uses the native table search form for archive indicator snapshots', () => {
  render(<ReportVersionIndicators version={versionFixture()} archiveTable />)
  expect(screen.getByRole('combobox', { name: /年级/ })).toBeVisible()
  expect(screen.getByRole('combobox', { name: /班级/ })).toBeVisible()
  expect(screen.getByRole('button', { name: /查\s*询/ })).toBeVisible()
  expect(screen.getByRole('button', { name: /重\s*置/ })).toBeVisible()
  expect(screen.getByText('一班')).toBeVisible()
  expect(screen.getByText('二班')).toBeVisible()
  expect(screen.queryByText('V1 周指标')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: /重\s*置/ }))
  expect(screen.getByText('一班')).toBeVisible()
})
