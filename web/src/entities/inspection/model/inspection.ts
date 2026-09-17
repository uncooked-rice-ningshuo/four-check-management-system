export type InspectionContext = {
  semesterId: string
  weekId: string
  inspectionTypeId: string
  departmentId: string
  classId?: string
  grade?: string
}

export type AttendanceRecord = {
  semesterId?: string
  weekId?: string
  inspectionTypeId?: string
  departmentId?: string
  createdBy?: string
  id: string
  studentNo: string
  studentName: string
  classId: string
  className: string
  grade: string
  courseId: string
  courseName: string
  classroomCode?: string
  occurredAt: string
  absenceHours: number
  period: string
  remark: string
  source: 'EXCEL' | 'MANUAL'
  status: string
  rowVersion: number
}

export type Page<T> = { items: T[]; total: number; page: number; pageSize: number }

export type AttendanceFilters = InspectionContext & {
  keyword?: string
	status?: 'DRAFT' | 'PENDING_REVIEW' | 'RETURNED' | 'CONFIRMED' | 'LOCKED'
	source?: 'MANUAL' | 'EXCEL' | 'REPORT_CORRECTION' | 'MIGRATION'
  sort?: 'occurredAtDesc' | 'occurredAtAsc' | 'studentNoAsc' | 'studentNoDesc'
  page: number
  pageSize: number
}

export type CurrentAcademicWeek = { id: string; semesterId: string; weekNo: number; startDate: string; endDate: string; enabled: boolean; note: string }

export type AttendanceFilterOptions = {
	inspectionType: { id: string; code: 'ATTENDANCE_RATE'; name: string }
	responsibleDepartments: { id: string; code: string; name: string; enabled: boolean }[]
	departments: { id: string; code: string; name: string; enabled: boolean }[]
	semesters: { id: string; academicYearId: string; code: string; name: string; startDate: string; endDate: string; enabled: boolean }[]
	weeks: CurrentAcademicWeek[]
	classes: { id: string; name: string; grade: string }[]
	grades: string[]
}

// ManualAttendanceInput uses business names; snapshots and state are server-owned.
export type ManualAttendanceInput = InspectionContext & {
  classId: string
  grade: string
  className: string
  studentNo: string
  studentName: string
  courseName: string
  classroom: string
  date: string
  time: string
  period: string
  absenceHours: number
  remark?: string
}
