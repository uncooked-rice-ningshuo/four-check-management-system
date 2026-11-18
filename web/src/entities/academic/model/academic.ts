export type YearRoster = { majorIds: string[]; grades: { gradeId: string; majors: { majorId: string; classIds: string[] }[] }[] }
export type AcademicYear = { roster?: YearRoster; version: number; stageGrades: Partial<Record<1 | 2 | 3 | 4, string>>; id: string; name: string; startDate: string; endDate: string; enabled: boolean }
export type Semester = { id: string; academicYearId: string; code: string; name: string; startDate: string; endDate: string; enabled: boolean }
export type SemesterWeek = { id: string; semesterId: string; weekNo: number; startDate: string; endDate: string; enabled: boolean; note: string; attendancePenaltyMultiplier: 1 | 2 | null }
export type AcademicCalendar = { academicYears: AcademicYear[]; semesters: Semester[]; weeks: SemesterWeek[] }
