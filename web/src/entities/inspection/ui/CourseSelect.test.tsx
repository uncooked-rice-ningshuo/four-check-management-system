import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { courseInspectionApi, type InspectionQuery } from '../../../shared/api/courseInspectionApi'
import { CourseSelect } from './CourseSelect'

vi.mock('../../../shared/api/courseInspectionApi', () => ({ courseInspectionApi: { courseOptions: vi.fn() } }))

const courseOptions = vi.mocked(courseInspectionApi.courseOptions)
const scope = { semesterId: 'semester', weekId: 'week', inspectionTypeId: 'type', departmentId: 'department', recordId: undefined }

function renderCourseSelect() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><CourseSelect scope={scope} /></QueryClientProvider>)
}

function page(page: number) {
  return {
    items: Array.from({ length: page === 1 ? 20 : 1 }, (_, index) => {
      const number = (page - 1) * 20 + index + 1
      return { id: `course-${number}`, label: `课程 ${number}` }
    }),
    total: 21,
    page,
    pageSize: 20,
  }
}

beforeEach(() => courseOptions.mockReset())

it('loads the next bounded course page when the option list scrolls to its end', async () => {
  courseOptions.mockImplementation(async (query: InspectionQuery = {}) => page(Number(query.page ?? 1)))
  renderCourseSelect()
  fireEvent.mouseDown(screen.getByRole('combobox'))
  expect(await screen.findByText('课程 1')).toBeInTheDocument()
  await waitFor(() => expect(courseOptions).toHaveBeenCalledWith(expect.objectContaining({ page: 1, pageSize: 20 })))

  const list = document.querySelector('.rc-virtual-list-holder')
  expect(list).not.toBeNull()
  fireEvent.scroll(list!, { target: { scrollTop: 1000 } })

  await waitFor(() => expect(courseOptions).toHaveBeenCalledWith(expect.objectContaining({ page: 2, pageSize: 20 })))
  expect(await screen.findByText('课程 21')).toBeInTheDocument()
})

it('caps a fuzzy search string at 128 Unicode characters', async () => {
  courseOptions.mockImplementation(async () => ({ ...page(1), total: 20 }))
  renderCourseSelect()
  const search = screen.getByRole('combobox')
  fireEvent.mouseDown(search)
  await screen.findByText('课程 1')
  fireEvent.change(search, { target: { value: '课'.repeat(160) } })

  await waitFor(() => expect(courseOptions).toHaveBeenLastCalledWith(expect.objectContaining({ keyword: '课'.repeat(128), page: 1, pageSize: 20 })), { timeout: 2000 })
})
