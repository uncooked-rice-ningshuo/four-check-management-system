import { apiRequest } from './httpClient'
import { badResponse, object } from './inspectionOptionsApi'

export type HygieneAreaKind = 'classroom-hygiene-areas' | 'public-dormitory-hygiene-areas'
export type HygieneAreaTarget = { kind: HygieneAreaKind; departmentId: string; departmentName: string; inspectionTypeId: string; inspectionTypeName: string }
export type HygieneArea = { id: string; kind: HygieneAreaKind; name: string; enabled: boolean; classroomId?: string; dormitoryBuildingId?: string; floorUnit?: string }
export type HygieneParent = { id: string; name: string; roomNo?: string }
export type HygienePage<T> = { items: T[]; total: number; page: number; pageSize: number }
export type HygieneChange = { id: string; action: string; actorId: string; createdAt: string; beforeJson?: unknown; afterJson?: unknown }

const base = '/admin/hygiene-areas'
const targetQuery = (departmentId: string) => `departmentId=${encodeURIComponent(departmentId)}`

function isKind(value: unknown): value is HygieneAreaKind {
  return value === 'classroom-hygiene-areas' || value === 'public-dormitory-hygiene-areas'
}
function isTarget(value: unknown): value is HygieneAreaTarget {
  return object(value) && isKind(value.kind) && ['departmentId', 'departmentName', 'inspectionTypeId', 'inspectionTypeName'].every(key => typeof value[key] === 'string' && value[key] !== '')
}
function isParent(value: unknown): value is HygieneParent {
  return object(value) && typeof value.id === 'string' && !!value.id && typeof value.name === 'string' && (value.roomNo === undefined || typeof value.roomNo === 'string')
}
function isArea(value: unknown): value is HygieneArea {
  if (!object(value) || typeof value.id !== 'string' || !value.id || typeof value.name !== 'string' || !isKind(value.kind) || typeof value.enabled !== 'boolean') return false
  return value.kind === 'classroom-hygiene-areas'
    ? typeof value.classroomId === 'string' && !!value.classroomId
    : typeof value.dormitoryBuildingId === 'string' && !!value.dormitoryBuildingId && typeof value.floorUnit === 'string' && !!value.floorUnit
}
function area(value: unknown): HygieneArea {
  if (!isArea(value)) throw badResponse()
  return value
}
function paged<T>(value: unknown, valid: (item: unknown) => item is T): HygienePage<T> {
  if (!object(value) || !Array.isArray(value.items) || !value.items.every(valid) || typeof value.total !== 'number' || !Number.isInteger(value.total) || value.total < 0 || typeof value.page !== 'number' || !Number.isInteger(value.page) || value.page < 1 || typeof value.pageSize !== 'number' || !Number.isInteger(value.pageSize) || value.pageSize < 1) throw badResponse()
  return { items: value.items, total: value.total, page: value.page, pageSize: value.pageSize }
}
export const hygieneAreaApi = {
  async targets() {
    const value = await apiRequest<unknown>(`${base}/options`)
    if (!Array.isArray(value) || !value.every(isTarget)) throw badResponse()
    return value
  },
  async list(kind: HygieneAreaKind, departmentId: string, page: number) {
    return paged(await apiRequest<unknown>(`${base}/${kind}?${targetQuery(departmentId)}&page=${page}&pageSize=20`), isArea)
  },
  async references(kind: HygieneAreaKind, departmentId: string) {
    const items: HygieneParent[] = []
    const seen = new Set<string>()
    for (let page = 1; ; page += 1) {
      const result = paged(await apiRequest<unknown>(`${base}/${kind}/references?${targetQuery(departmentId)}&page=${page}`), isParent)
      if (result.page !== page) throw badResponse()
      for (const item of result.items) {
        if (seen.has(item.id)) throw badResponse()
        seen.add(item.id)
        items.push(item)
      }
      if (items.length === result.total) return items
      if (!result.items.length || items.length > result.total) throw badResponse()
    }
  },
  create: async (kind: HygieneAreaKind, departmentId: string, input: Omit<HygieneArea, 'id' | 'kind'>) => area(await apiRequest<unknown>(`${base}/${kind}?${targetQuery(departmentId)}`, { method: 'POST', body: JSON.stringify(input) })),
  update: async (kind: HygieneAreaKind, departmentId: string, input: HygieneArea) => area(await apiRequest<unknown>(`${base}/${kind}/${encodeURIComponent(input.id)}?${targetQuery(departmentId)}`, { method: 'PUT', body: JSON.stringify(input) })),
  changes: (kind: HygieneAreaKind, departmentId: string, id: string) => apiRequest<HygieneChange[]>(`${base}/${kind}/${encodeURIComponent(id)}/changes?${targetQuery(departmentId)}`),
}
