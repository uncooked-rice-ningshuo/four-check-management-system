import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { PhaseTwoDashboard } from './PhaseTwoDashboard'

const dashboardQuery = vi.fn()
vi.mock('../model/usePhaseTwoDashboard', () => ({ usePhaseTwoDashboard: () => dashboardQuery() }))

describe('PhaseTwoDashboard', () => {
	beforeEach(() => {
		dashboardQuery.mockReturnValue({
			isLoading: false,
			isError: false,
			data: {
				context: { academicYearName: '2026-2027 学年', semesterName: '第一学期', weekNo: 2 },
				myTodos: { total: 2, draft: 1, returned: 1 },
				departmentStatuses: [{
					departmentId: 'study', departmentName: '学习部', inspectionTypeId: 'attendance',
					inspectionTypeCode: 'ATTENDANCE_RATE', inspectionTypeName: '到课率', status: 'PENDING_REVIEW',
					total: 9, draft: 2, pendingReview: 3, returned: 0, confirmed: 4, locked: 0,
					batchTotal: 3, batchPending: 1, batchReturned: 1, batchConfirmed: 1,
					canEnter: true, canReview: false,
				}],
			},
		})
	})

	it('keeps personal todo counts separate from department-wide progress', () => {
		render(<MemoryRouter><PhaseTwoDashboard userName="委员甲" /></MemoryRouter>)

		expect(screen.getByRole('heading', { name: '工作台' })).toBeInTheDocument()
		expect(screen.getByText('我的待办')).toBeInTheDocument()
		expect(screen.getByText('2', { selector: '.ant-statistic-content-value-int' })).toBeInTheDocument()
		expect(screen.getByText('学习部')).toBeInTheDocument()
		expect(screen.getByText('部门整体')).toBeInTheDocument()
		expect(screen.getByText((_, element) => element?.textContent === '9 条记录')).toBeInTheDocument()
		expect(screen.getByText((_, element) => element?.textContent === '3 个提交批次')).toBeInTheDocument()
		expect(screen.getByRole('link', { name: '查看到课率' })).toHaveAttribute('href', '/inspections/attendance')
	})

	it('shows a truthful empty context instead of zero-filled fake progress', () => {
		dashboardQuery.mockReturnValue({ isLoading: false, isError: false, data: { context: null, myTodos: { total: 0, draft: 0, returned: 0 }, departmentStatuses: [] } })
		render(<MemoryRouter><PhaseTwoDashboard userName="委员甲" /></MemoryRouter>)
		expect(screen.getByText('当前日期未匹配到已启用的学期周次')).toBeInTheDocument()
		expect(screen.queryByText('0 个部门已完成')).not.toBeInTheDocument()
	})

	it('contains no phase-three report placeholders', () => {
		render(<MemoryRouter><PhaseTwoDashboard userName="委员甲" /></MemoryRouter>)
		for (const label of ['待生成通报', '待通报审核', '待定稿', '全院通报进度']) {
			expect(screen.queryByText(label)).not.toBeInTheDocument()
		}
	})

	it('links each active check type without adding deferred hygiene shortcuts', () => {
		const current = dashboardQuery().data
		const sample = current.departmentStatuses[0]
		const types: Array<[string, string, string]> = [
			['ATTENDANCE_RATE', '到课率', '/inspections/attendance'],
			['UNATTENDED_CLASSROOM', '无机课堂', '/inspections/unattended-classroom'],
			['MORNING_ATTENDANCE', '早锻炼/早签', '/inspections/morning-attendance'],
			['DORMITORY_HYGIENE', '宿舍卫生', '/inspections/dormitory-hygiene'],
			['LATE_RETURN', '晚归', '/inspections/late-return'],
			['CLASSROOM_HYGIENE', '教室卫生', ''], ['PUBLIC_DORMITORY_HYGIENE', '公共宿舍卫生', ''],
		]
		dashboardQuery.mockReturnValue({ isLoading: false, isError: false, data: { ...current,
			departmentStatuses: types.map(([inspectionTypeCode, inspectionTypeName], index) => ({
				...sample, inspectionTypeId: String(index), inspectionTypeCode, inspectionTypeName,
			})) } })
		render(<MemoryRouter><PhaseTwoDashboard /></MemoryRouter>)
		for (const [, name, route] of types.slice(0, 5)) {
			expect(screen.getByRole('link', { name: `查看${name}` })).toHaveAttribute('href', route)
		}
		expect(screen.queryByRole('link', { name: '查看教室卫生' })).not.toBeInTheDocument()
		expect(screen.queryByRole('link', { name: '查看公共宿舍卫生' })).not.toBeInTheDocument()
	})
})
