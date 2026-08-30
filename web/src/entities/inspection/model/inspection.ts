export type InspectionContext = {
  semesterId: string
  weekId: string
  inspectionTypeId: string
  departmentId: string
  classId?: string
  grade?: string
}

export type AttendanceRecord = {
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
  sort?: 'occurredAtDesc' | 'occurredAtAsc' | 'studentNoAsc' | 'studentNoDesc'
  page: number
  pageSize: number
}

export type CurrentAcademicWeek = { id: string; semesterId: string; weekNo: number; startDate: string; endDate: string; enabled: boolean; note: string }
