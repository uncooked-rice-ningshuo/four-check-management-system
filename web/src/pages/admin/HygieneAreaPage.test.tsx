import { StrictMode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { hygieneAreaApi, type HygieneArea } from '../../shared/api/hygieneAreaApi'
import { HygieneAreaPage } from './HygieneAreaPage'

vi.mock('../../shared/api/hygieneAreaApi', () => ({ hygieneAreaApi: {
  targets: vi.fn(), list: vi.fn(), references: vi.fn(), changes: vi.fn(), create: vi.fn(), update: vi.fn(),
} }))
const areas: HygieneArea[] = [
  { id: 'a', kind: 'classroom-hygiene-areas', name: '前排', classroomId: 'room-a', enabled: true },
  { id: 'b', kind: 'classroom-hygiene-areas', name: '后排', classroomId: 'room-b', enabled: true },
]
beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(hygieneAreaApi.targets).mockResolvedValue([{ kind: 'classroom-hygiene-areas', departmentId: 'd', departmentName: '检查部门', inspectionTypeId: 't', inspectionTypeName: '教室卫生' }])
  vi.mocked(hygieneAreaApi.list).mockResolvedValue({ items: areas, total: 2, page: 1, pageSize: 20 })
  vi.mocked(hygieneAreaApi.references).mockResolvedValue([{ id: 'room-a', name: '第一教室' }, { id: 'room-b', name: '第二教室' }])
  vi.mocked(hygieneAreaApi.changes).mockResolvedValue([])
  vi.mocked(hygieneAreaApi.update).mockResolvedValue(areas[0])
})
function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><StrictMode><HygieneAreaPage /></StrictMode></QueryClientProvider>)
  return client
}
async function closeEditor() {
  fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Close' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
}
it('首次编辑、新增关闭后编辑、A 到 B 和关闭重开均正确回显', async () => {
  setup()
  await screen.findByRole('cell', { name: '第一教室 / 前排' })
  for (const index of [0, 1, 0]) {
    fireEvent.click(screen.getAllByRole('button', { name: '编辑' })[index])
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByLabelText('区域名称')).toHaveValue(areas[index].name)
    fireEvent.change(within(dialog).getByLabelText('区域名称'), { target: { value: '未保存输入' } })
    await closeEditor()
  }
  fireEvent.click(screen.getByRole('button', { name: '新增区域' }))
  expect(within(await screen.findByRole('dialog')).getByLabelText('区域名称')).toHaveValue('')
  await closeEditor()
  fireEvent.click(screen.getAllByRole('button', { name: '编辑' })[1])
  expect(within(await screen.findByRole('dialog')).getByLabelText('区域名称')).toHaveValue('后排')
})
it('失败保留输入可重试，关闭后不将错误带入下一次编辑', async () => {
  vi.mocked(hygieneAreaApi.update).mockRejectedValueOnce(new Error('服务失败'))
  setup()
  fireEvent.click((await screen.findAllByRole('button', { name: '编辑' }))[0])
  const dialog = await screen.findByRole('dialog')
  fireEvent.change(within(dialog).getByLabelText('区域名称'), { target: { value: '新区域名称' } })
  fireEvent.click(within(dialog).getByRole('button', { name: /保\s*存/ }))
  await screen.findByText('保存失败，请检查输入或权限后重试')
  expect(within(dialog).getByLabelText('区域名称')).toHaveValue('新区域名称')
  await closeEditor()
  fireEvent.click(screen.getAllByRole('button', { name: '编辑' })[1])
  await screen.findByRole('dialog')
  expect(screen.queryByText('保存失败，请检查输入或权限后重试')).not.toBeInTheDocument()
})
it('权限选项失败有重试提示且不展示无权限空态', async () => {
  vi.mocked(hygieneAreaApi.targets).mockRejectedValueOnce(new Error('服务失败'))
  setup()
  expect(await screen.findByText('区域配置加载失败，请重试')).toBeInTheDocument()
  expect(screen.queryByText('当前账号没有卫生区域维护权限')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: /重\s*试/ }))
  expect(await screen.findByRole('cell', { name: '第一教室 / 前排' })).toBeInTheDocument()
})
it('成功保存刷新已有业务关联选项和变更历史缓存', async () => {
  const client = setup()
  client.setQueryData(['course-inspections', 'entry-options', 'area'], [{ id: 'a', label: '旧名' }])
  client.setQueryData(['hygiene-area-changes', 'classroom-hygiene-areas', 'd', 'a'], [])
  fireEvent.click((await screen.findAllByRole('button', { name: '编辑' }))[0])
  const dialog = await screen.findByRole('dialog')
  fireEvent.change(within(dialog).getByLabelText('区域名称'), { target: { value: '新名' } })
  fireEvent.click(within(dialog).getByRole('button', { name: /保\s*存/ }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(client.getQueryState(['course-inspections', 'entry-options', 'area'])?.isInvalidated).toBe(true)
  expect(client.getQueryState(['hygiene-area-changes', 'classroom-hygiene-areas', 'd', 'a'])?.isInvalidated).toBe(true)
})
