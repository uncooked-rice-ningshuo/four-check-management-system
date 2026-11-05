import type { ReportingRecord } from '../../entities/reporting/model/reporting'

/** 到课率的缺课数量按节次展示；无机课堂每天一条记录，按违纪次数展示，不换算学时。 */
export function absenceSummary(record: ReportingRecord): string | undefined {
  if (record.typeCode === 'UNATTENDED_CLASSROOM') return undefined
  if (record.details.absenceHours === undefined) return undefined
  return record.typeCode === 'ATTENDANCE_RATE'
    ? `缺课 ${record.details.absenceHours} 节次`
    : `缺课 ${record.details.absenceHours} 小时`
}
