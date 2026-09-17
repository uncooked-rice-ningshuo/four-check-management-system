import { apiRequest } from '../../../shared/api/httpClient'

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
		canEnter: boolean
		canReview: boolean
	}>
}

export function getPhaseTwoDashboard() {
	return apiRequest<PhaseTwoDashboardData>('/dashboard/workbench')
}
