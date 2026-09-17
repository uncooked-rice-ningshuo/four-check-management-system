import type { SemesterWeek } from '../../academic/model/academic'
export type WorkContext = { week: SemesterWeek | null; source: 'CALENDAR' | 'OVERRIDE' | 'NONE'; version: number; expiresAt?: string }
export type WeekOverrideInput = { weekId: string; expiresAt: string | null; version: number; reason: string }
