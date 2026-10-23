import { useQuery } from '@tanstack/react-query'
import { allOptions, type InspectionQuery } from '../../../shared/api/inspectionOptionsApi'
import type { SupportedImportType } from '../../../shared/api/inspectionImportApi'

/** Load complete, permission-filtered destinations independently of list filters. */
export function useImportContext(query: InspectionQuery, enabled: boolean, typeCode: SupportedImportType) {
  const root = typeCode === 'CLASSROOM_HYGIENE' ? 'classroom-hygienes' : typeCode === 'DORMITORY_HYGIENE' ? 'dormitory-hygienes' : typeCode === 'LATE_RETURN' ? 'late-returns' : typeCode === 'MORNING_ATTENDANCE' ? 'morning-attendances' : 'course-inspections'
  return useQuery({ queryKey: [root, 'import-context', typeCode, query], queryFn: () => allOptions('/inspections/context-options', { ...query, typeCode }, true), enabled })
}
