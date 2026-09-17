import { apiRequest } from './httpClient'
import type { WorkContext, WeekOverrideInput } from '../../entities/work-context/model/workContext'
export const getWorkContext = () => apiRequest<WorkContext>('/work-context')
export const setWorkContext = (input: WeekOverrideInput) => apiRequest<WorkContext>('/work-context', { method: 'PUT', body: JSON.stringify(input) })
