import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getCatalogs, getUsers } from './adminApi'
import { apiRequest } from './httpClient'

vi.mock('./httpClient', async original => ({ ...await original<typeof import('./httpClient')>(), apiRequest: vi.fn() }))
const request = vi.mocked(apiRequest)
const user = { accountId: 'user', loginName: '001', studentNo: '001', name: '示例用户', status: 'ACTIVE', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false, departmentId: '', organizationRoleId: '' }
const department = { id: 'department', code: 'TEST', name: '示例部门', enabled: true }
const role = { id: 'role', code: 'MEMBER', name: '委员', enabled: true, sortOrder: 1 }
const inspection = { id: 'inspection', code: 'ATTENDANCE', name: '到课率', extensionKind: 'ATTENDANCE', enabled: true, participatesInReport: true, sortOrder: 1 }
const catalogs = { departments: [department], organizationRoles: [role], inspectionTypes: [inspection] }

beforeEach(() => request.mockReset())

describe('admin read response contracts', () => {
  it('accepts complete and genuinely empty responses', async () => {
    request.mockResolvedValueOnce([user]).mockResolvedValueOnce([]).mockResolvedValueOnce(catalogs).mockResolvedValueOnce({ departments: [], organizationRoles: [], inspectionTypes: [] })
    expect(await getUsers()).toEqual([user])
    expect(await getUsers()).toEqual([])
    expect(await getCatalogs()).toEqual(catalogs)
    expect(await getCatalogs()).toEqual({ departments: [], organizationRoles: [], inspectionTypes: [] })
  })

  it.each([null, {}, [null], [{ ...user, name: undefined }], [{ ...user, isSuperAdmin: 'false' }], [{ ...user, systemRole: 'UNKNOWN' }], [{ ...user, accountId: '' }], [user, user]])('rejects malformed users instead of creating an empty or misleading list: %j', async value => {
    request.mockResolvedValue(value)
    await expect(getUsers()).rejects.toMatchObject({ status: 502, code: 'INVALID_RESPONSE' })
  })

  it.each([null, [], {}, { ...catalogs, departments: null }, { ...catalogs, organizationRoles: undefined }, { ...catalogs, departments: [{ ...department, enabled: 'false' }] }, { ...catalogs, organizationRoles: [{ ...role, sortOrder: '1' }] }, { ...catalogs, inspectionTypes: [{ ...inspection, participatesInReport: undefined }] }, { ...catalogs, departments: [department, department] }])('rejects malformed catalogs: %j', async value => {
    request.mockResolvedValue(value)
    await expect(getCatalogs()).rejects.toMatchObject({ status: 502, code: 'INVALID_RESPONSE' })
  })

  it('preserves request failures and accepts extra fields for forward compatibility', async () => {
    const failure = new Error('request failed')
    request.mockRejectedValueOnce(failure).mockResolvedValueOnce([{ ...user, futureField: true }])
    await expect(getUsers()).rejects.toBe(failure)
    expect(await getUsers()).toEqual([expect.objectContaining(user)])
  })
})
