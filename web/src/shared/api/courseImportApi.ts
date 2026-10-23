import { createInspectionImportClient } from './inspectionImportApi'
export type { InspectionImportScope as CourseImportScope } from './inspectionImportApi'
const client = createInspectionImportClient('/imports/course-records')
/** Keep existing course endpoint consumers compatible with the generic client. */
export const courseImportApi = { template: client.template, uploadAttendance: client.upload }
