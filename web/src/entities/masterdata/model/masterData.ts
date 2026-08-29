export type MasterDataKind = 'faculties' | 'majors' | 'staff' | 'classes' | 'dormitories' | 'classrooms' | 'courses' | 'students'

export type MasterDataRecord = {
  id: string
  kind: MasterDataKind
  code: string
  name: string
  enabled: boolean
  facultyId?: string
  majorId?: string
  headTeacherId?: string
  teacherId?: string
  classId?: string
  dormitoryId?: string
  classroomId?: string
  gender?: string
  phone?: string
  grade?: string
  enrolledSize?: number
  campusArea?: string
  building?: string
  buildingNo?: string
  roomNo?: string
  floor?: number
  scheduleText?: string
  enrollmentStatus?: string
}

export type MasterDataPage = { items: MasterDataRecord[]; total: number; page: number; pageSize: number }
export type MasterDataChange = { id: string; entityType: string; entityId: string; action: string; beforeJson?: unknown; afterJson?: unknown; actorId: string; createdAt: string }
