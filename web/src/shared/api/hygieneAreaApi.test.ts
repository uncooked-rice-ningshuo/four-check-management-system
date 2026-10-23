import { beforeEach, expect, it, vi } from 'vitest'
import { apiRequest } from './httpClient'
import { hygieneAreaApi } from './hygieneAreaApi'
vi.mock('./httpClient', async importOriginal => ({ ...await importOriginal<typeof import('./httpClient')>(), apiRequest: vi.fn() }))
beforeEach(() => vi.resetAllMocks())
it('关联对象覆盖所有分页', async () => {
  vi.mocked(apiRequest).mockResolvedValueOnce({ items: [{ id: 'a', name: '教学楼 101' }], total: 2, page: 1, pageSize: 1 }).mockResolvedValueOnce({ items: [{ id: 'b', name: '教学楼 102' }], total: 2, page: 2, pageSize: 1 })
  expect(await hygieneAreaApi.references('classroom-hygiene-areas', 'd')).toEqual([{ id: 'a', name: '教学楼 101' }, { id: 'b', name: '教学楼 102' }])
  expect(apiRequest).toHaveBeenLastCalledWith('/admin/hygiene-areas/classroom-hygiene-areas/references?departmentId=d&page=2')
})
it('不完整分页必须失败，不能展示正常空选项', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ items: [], total: 1, page: 1, pageSize: 100 })
  await expect(hygieneAreaApi.references('classroom-hygiene-areas', 'd')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})
it('修改携带明确部门目标、区域类型和资源标识', async () => {
  const value = { id: 'area', kind: 'public-dormitory-hygiene-areas' as const, name: '走廊', enabled: true, dormitoryBuildingId: 'building', floorUnit: '1' }
  vi.mocked(apiRequest).mockResolvedValue(value)
  await hygieneAreaApi.update(value.kind, 'd', value)
  expect(apiRequest).toHaveBeenCalledWith('/admin/hygiene-areas/public-dormitory-hygiene-areas/area?departmentId=d', { method: 'PUT', body: JSON.stringify(value) })
})
it.each([null, {}, [{ kind: 'wrong' }]])('维护范围异常响应必须明确失败：%j', async value => {
  vi.mocked(apiRequest).mockResolvedValue(value)
  await expect(hygieneAreaApi.targets()).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})
it('缺少列表 items 时不能呈现正常空状态', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ total: 0, page: 1, pageSize: 20 })
  await expect(hygieneAreaApi.list('classroom-hygiene-areas', 'd', 1)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})
it('重复的关联分页必须失败而非用重复选项补足数量', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ items: [{ id: 'a', name: '教学楼 101' }], total: 2, page: 1, pageSize: 1 })
  await expect(hygieneAreaApi.references('classroom-hygiene-areas', 'd')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})
