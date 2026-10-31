import type { ReportingRecord } from '../../entities/reporting/model/reporting'

/** 到课率的缺课数量按节次展示。 */
export function absenceSummary(record: ReportingRecord): string | undefined {
  if (record.details.absenceHours === undefined) return undefined
  return record.typeCode === 'ATTENDANCE_RATE'
    ? `缺课 ${record.details.absenceHours} 节次`
    : `缺课 ${record.details.absenceHours} 小时`
}
