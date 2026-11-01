import { apiRequest } from './httpClient'

export type MorningSchedule = {
  semesterId: string
  weekId: string
  startDate: string
  endDate: string
  plannedDates: string[]
  version: number
  updatedBy?: string
  updatedAt?: string
  isDefault: boolean
  canEdit: boolean
}

function parse(value: unknown): MorningSchedule {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('早签计划响应格式无效')
  const item = value as Record<string, unknown>
  if (typeof item.semesterId !== 'string' || typeof item.weekId !== 'string' || typeof item.startDate !== 'string' || typeof item.endDate !== 'string' ||
    !Array.isArray(item.plannedDates) || !item.plannedDates.every(date => typeof date === 'string') || !Number.isInteger(item.version) ||
    typeof item.isDefault !== 'boolean' || typeof item.canEdit !== 'boolean') throw new Error('早签计划响应格式无效')
  return item as MorningSchedule
}

export const morningScheduleApi = {
  async get(weekId: string): Promise<MorningSchedule> {
    return parse(await apiRequest<unknown>(`/inspections/morning-schedule/${encodeURIComponent(weekId)}`))
  },
  async save(weekId: string, version: number, plannedDates: string[]): Promise<MorningSchedule> {
    return parse(await apiRequest<unknown>(`/inspections/morning-schedule/${encodeURIComponent(weekId)}`, {
      method: 'PUT', body: JSON.stringify({ version, plannedDates }),
    }))
  },
}
