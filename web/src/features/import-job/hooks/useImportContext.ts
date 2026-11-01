import { useQuery } from '@tanstack/react-query'
import { allOptions, type InspectionQuery } from '../../../shared/api/inspectionOptionsApi'
import type { SupportedImportType } from '../../../shared/api/inspectionImportApi'

/** Load complete, permission-filtered destinations independently of list filters. */
export function useImportContext(query: InspectionQuery, enabled: boolean, typeCode: SupportedImportType) {
  const rootByType: Record<SupportedImportType, string> = {
    ATTENDANCE_RATE: 'attendance',
    UNATTENDED_CLASSROOM: 'course-inspections',
    MORNING_ATTENDANCE: 'morning-attendances',
    LATE_RETURN: 'late-returns',
    DORMITORY_HYGIENE: 'dormitory-hygienes',
    CLASSROOM_HYGIENE: 'classroom-hygienes',
    PUBLIC_DORMITORY_HYGIENE: 'public-dormitory-hygienes',
    EVENING_SELF_STUDY: 'evening-self-studies',
  }
  const root = rootByType[typeCode]
  return useQuery({ queryKey: [root, 'import-context', typeCode, query], queryFn: () => allOptions('/inspections/context-options', { ...query, typeCode }, true), enabled })
}
