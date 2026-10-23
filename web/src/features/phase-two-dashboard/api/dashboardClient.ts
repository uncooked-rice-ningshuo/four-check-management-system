import { ApiError, apiRequest } from '../../../shared/api/httpClient'

export type DepartmentProgressStatus = 'NOT_ENTERED' | 'ENTERING' | 'PENDING_REVIEW' | 'RETURNED' | 'COMPLETED'

export type PhaseTwoDashboardData = {
	context: null | {
		academicYearId?: string
		academicYearName?: string
		semesterId: string
		semesterName?: string
		weekId: string
		weekNo: number
		startDate: string
		endDate: string
	}
	myTodos: { total: number; draft: number; returned: number }
	departmentStatuses: Array<{
		departmentId: string
		departmentName: string
		inspectionTypeId: string
		inspectionTypeCode: string
		inspectionTypeName: string
		status: DepartmentProgressStatus
		total: number
		draft: number
		pendingReview: number
		returned: number
		confirmed: number
		locked: number
		batchTotal: number
		batchPending: number
		batchReturned: number
		batchConfirmed: number
		canEnter: boolean
		canReview: boolean
	}>
}

function isObject(value: unknown): value is Record<string, unknown> {
	return value !== null && typeof value === 'object' && !Array.isArray(value)
}

function validatedDashboard(value: unknown): PhaseTwoDashboardData {
	const invalid = () => new ApiError(502, 'INVALID_RESPONSE', '工作台数据不完整，请刷新或联系管理员。')
	if (!isObject(value) || !isObject(value.myTodos) || !Array.isArray(value.departmentStatuses) ||
		(value.context !== null && !isObject(value.context))) throw invalid()
	if (value.context !== null) {
		const context = value.context as Record<string, unknown>
		for (const key of ['semesterId', 'weekId', 'startDate', 'endDate']) if (typeof context[key] !== 'string') throw invalid()
		if (!Number.isInteger(context.weekNo)) throw invalid()
	}
	for (const key of ['total', 'draft', 'returned']) if (!Number.isInteger(value.myTodos[key])) throw invalid()
	for (const item of value.departmentStatuses) {
		if (!isObject(item)) throw invalid()
		for (const key of ['departmentId', 'departmentName', 'inspectionTypeId', 'inspectionTypeCode', 'inspectionTypeName', 'status']) {
			if (typeof item[key] !== 'string') throw invalid()
		}
		for (const key of ['total', 'draft', 'pendingReview', 'returned', 'confirmed', 'locked',
			'batchTotal', 'batchPending', 'batchReturned', 'batchConfirmed']) if (!Number.isInteger(item[key])) throw invalid()
		if (typeof item.canEnter !== 'boolean' || typeof item.canReview !== 'boolean') throw invalid()
	}
	return value as PhaseTwoDashboardData
}

export async function getPhaseTwoDashboard() {
	return validatedDashboard(await apiRequest<unknown>('/dashboard/workbench'))
}
