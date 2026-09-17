export type MasterDataKind = 'faculties' | 'grades' | 'majors' | 'staff' | 'classes' | 'dormitory-areas' | 'dormitory-buildings' | 'dormitories' | 'teaching-buildings' | 'classrooms' | 'courses' | 'students'

export type MasterDataRecord = {
  id: string
  kind: MasterDataKind
  code?: string
  name: string
  enabled: boolean
  facultyId?: string
  majorId?: string
  headTeacherId?: string
  teacherId?: string
  classId?: string
  dormitoryId?: string
  classroomId?: string
  teachingBuildingId?: string
  dormitoryAreaId?: string
  dormitoryBuildingId?: string
  gender?: string
  phone?: string
  grade?: string
  gradeId?: string
  classIds?: string[]
  schedules?: CourseSchedule[]
  enrolledSize?: number
  campusArea?: string
  buildingNo?: string
  roomNo?: string
  scheduleText?: string
  enrollmentStatus?: string
}

export type CourseSchedule = {
  id?: string
  semesterId: string
  classroomId: string
  weekday: number
  startSection: number
  endSection: number
}

export type MasterDataPage = { items: MasterDataRecord[]; total: number; page: number; pageSize: number }
export type MasterDataChange = { id: string; entityType: string; entityId: string; action: string; beforeJson?: unknown; afterJson?: unknown; actorId: string; createdAt: string }
