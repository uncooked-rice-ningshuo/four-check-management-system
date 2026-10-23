import { ApiError, apiRequest } from './httpClient'
export type InspectionOption = { id: string; label: string; grade?: string; attendancePenaltyMultiplier?: 1 | 2 }
export type InspectionQuery = Record<string, string | number | undefined>
export type ContextOptions = { items: InspectionOption[]; inspectionTypeId: string }
export const badResponse = () => new ApiError(502, 'INVALID_RESPONSE', '服务返回的数据不完整，请刷新或联系管理员。')
export function object(value: unknown): value is Record<string, unknown> { return !!value && typeof value === 'object' && !Array.isArray(value) }
function option(value: unknown): value is InspectionOption { return object(value) && typeof value.id === 'string' && typeof value.label === 'string' && (value.grade === undefined || typeof value.grade === 'string') && (value.attendancePenaltyMultiplier === undefined || value.attendancePenaltyMultiplier === 1 || value.attendancePenaltyMultiplier === 2) }
export function queryString(query: InspectionQuery) {
 const params = new URLSearchParams()
 for (const [key,value] of Object.entries(query)) if (value !== undefined && value !== '') params.set(key,String(value))
 return params.toString()
}
export async function allOptions(path: string, query: InspectionQuery, context: boolean): Promise<ContextOptions> {
 const items: InspectionOption[] = [], seen = new Set<string>()
 let inspectionTypeId = ''
 for (let page=1; page<=1000000; page++) {
  const value = await apiRequest<unknown>(`${path}?${queryString({...query,page,pageSize:200})}`)
  if (!object(value) || !Array.isArray(value.items) || !value.items.every(option) || !Number.isInteger(value.total) || typeof value.total !== 'number' || value.total<0 || value.page!==page) throw badResponse()
  if (context) {
   if (typeof value.inspectionTypeId !== 'string' || !value.inspectionTypeId || (inspectionTypeId && inspectionTypeId!==value.inspectionTypeId)) throw badResponse()
   inspectionTypeId=value.inspectionTypeId
  }
  for (const item of value.items) { if(seen.has(item.id)) throw badResponse(); seen.add(item.id);items.push(item) }
  if(items.length===value.total) return {items,inspectionTypeId}
  if(!value.items.length || items.length>value.total) throw badResponse()
 }
 throw badResponse()
}
