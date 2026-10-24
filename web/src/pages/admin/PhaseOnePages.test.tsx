import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { Modal, message } from 'antd'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RegisterPage } from '../auth/RegisterPage'
import { RegisterResultPage } from '../auth/RegisterResultPage'
import { DictionaryPage } from './DictionaryPage'
import { MasterDataPage } from './MasterDataPage'
import { OrganizationConfigPage } from './OrganizationConfigPage'
import { RegistrationReviewPage } from './RegistrationReviewPage'
import { UserManagementPage } from './UserManagementPage'
import type { MasterDataImportBatch, MasterDataKind, MasterDataRecord, MasterDataImportSpec } from '../../entities/masterdata/model/masterData'
import { assignUserRole, confirmMasterDataImportBatch, createMasterData, downloadMasterDataImportTemplate, getAcademicCalendar, getCatalogs, getDepartmentInspectionScopes, getDictionaries, getEffectivePermissions, getMasterData, getMasterDataImportSpec, getPendingRegistrations, getPermissionRules, getUsers, previewMasterDataImport, resetUserPassword, reviewRegistration, updateDepartment, updateDepartmentInspectionScope, updateDictionaryItem, updateInspectionType, updateUser } from '../../shared/api/adminApi'

vi.mock('../../shared/api/adminApi', () => ({
  getRegistrationOptions: vi.fn().mockResolvedValue({ departments: [], organizationRoles: [], inspectionTypes: [] }),
  getPendingRegistrations: vi.fn().mockResolvedValue([]),
  reviewRegistration: vi.fn(),
  getUsers: vi.fn().mockResolvedValue([]),
  getCatalogs: vi.fn().mockResolvedValue({ departments: [], organizationRoles: [], inspectionTypes: [] }),
	getDepartmentInspectionScopes: vi.fn().mockResolvedValue([]),
	createDepartment: vi.fn(), updateDepartment: vi.fn(), updateInspectionType: vi.fn(), updateDepartmentInspectionScope: vi.fn(),
  createUser: vi.fn(), updateUser: vi.fn(), updateUserStatus: vi.fn(), resetUserPassword: vi.fn(), assignUserRole: vi.fn(),
  getEffectivePermissions: vi.fn().mockResolvedValue({ accountId: '', systemRole: 'MEMBER', permissions: [] }),
  getPermissionRules: vi.fn().mockResolvedValue([]), createPermissionRule: vi.fn(), updatePermissionRule: vi.fn(), deletePermissionRule: vi.fn(),
	getAcademicCalendar: vi.fn().mockResolvedValue({ academicYears: [], semesters: [], weeks: [] }),
  getMasterData: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 }),
  createMasterData: vi.fn(), updateMasterData: vi.fn(), getMasterDataChanges: vi.fn().mockResolvedValue([]),
  getMasterDataImportSpec: vi.fn(), downloadMasterDataImportTemplate: vi.fn(), previewMasterDataImport: vi.fn(), getMasterDataImportBatch: vi.fn(), confirmMasterDataImportBatch: vi.fn(),
  getDictionaries: vi.fn().mockResolvedValue({ types: [], items: [] }),
  createDictionaryType: vi.fn(), updateDictionaryType: vi.fn(), createDictionaryItem: vi.fn(), updateDictionaryItem: vi.fn(),
}))

vi.mock('../../shared/api/authApi', () => ({ registerAccount: vi.fn() }))

const renderPage = (element: React.ReactNode) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return { ...render(<QueryClientProvider client={client}><MemoryRouter>{element}</MemoryRouter></QueryClientProvider>), client }
}

describe('阶段一管理页面', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getUsers).mockResolvedValue([])
    vi.mocked(getPendingRegistrations).mockResolvedValue([])
    vi.mocked(getCatalogs).mockResolvedValue({ departments: [], organizationRoles: [], inspectionTypes: [] })
	vi.mocked(getDepartmentInspectionScopes).mockResolvedValue([])
    vi.mocked(getMasterData).mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
    vi.mocked(getMasterDataImportSpec).mockResolvedValue({ kind: 'students', templateCode: 'MASTER_DATA_STUDENTS', templateVersion: 1, sheetName: '学生导入', label: '学生', fields: [] })
    vi.mocked(getDictionaries).mockResolvedValue({ types: [], items: [] })
  })

	afterEach(() => Modal.destroyAll())

  it('注册协议使用 Ant Design Checkbox', async () => {
    const { container } = renderPage(<RegisterPage />)
    const checkbox = await screen.findByRole('checkbox', { name: /信息真实准确/ })
    expect(checkbox.closest('.ant-checkbox-wrapper')).not.toBeNull()
    expect(container.querySelector('label > input[type="checkbox"]')).toBeNull()
  })

  it('点击注册结果页按钮的任意区域都返回登录页', () => {
    render(<MemoryRouter initialEntries={['/register/result']}><Routes><Route path="/register/result" element={<RegisterResultPage />} /><Route path="/login" element={<div>登录页已打开</div>} /></Routes></MemoryRouter>)
    fireEvent.click(screen.getByRole('button', { name: '返回登录' }))
    expect(screen.getByText('登录页已打开')).toBeVisible()
  })

  it('注册审核失败时展示后端返回的错误消息', async () => {
    vi.mocked(getPendingRegistrations).mockResolvedValue([{ id: 'request-1', loginName: '24010001', studentNo: '24010001', name: '张三', status: 'PENDING', accountKind: 'PERSON', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false, primaryDepartmentId: '', requestedDepartmentId: 'department-1', requestedOrgRoleId: 'role-1' }])
    vi.mocked(reviewRegistration).mockRejectedValue(new Error('提交信息不符合要求'))
    renderPage(<RegistrationReviewPage />)
    fireEvent.click(await screen.findByRole('button', { name: /通.*过/ }))
    expect(await screen.findByText('提交信息不符合要求')).toBeVisible()
  })

  it('注册申请表格使用 ProTable 原生查询区筛选待审核账号', async () => {
    vi.mocked(getPendingRegistrations).mockResolvedValue([{ id: 'request-1', loginName: '24010001', studentNo: '24010001', name: '张三', status: 'PENDING', accountKind: 'PERSON', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false, primaryDepartmentId: '', requestedDepartmentId: 'department-1', requestedOrgRoleId: 'role-1' }])
    vi.mocked(getCatalogs).mockResolvedValue({
      departments: [{ id: 'department-1', code: 'ONE', name: '部门一', enabled: true }],
      organizationRoles: [{ id: 'role-1', code: 'MEMBER', name: '委员', enabled: true, sortOrder: 1 }],
      inspectionTypes: [],
    })
    const { container } = renderPage(<RegistrationReviewPage />)

    await screen.findByRole('cell', { name: '张三' })
    expect(container.querySelector('.ant-pro-query-filter')).not.toBeNull()
    expect(screen.getByLabelText('申请部门')).toBeVisible()
    expect(screen.getByLabelText('申请角色')).toBeVisible()
    fireEvent.change(screen.getByPlaceholderText('输入学号或姓名…'), { target: { value: '李四' } })
    fireEvent.click(screen.getByRole('button', { name: /查.*询/ }))

    await waitFor(() => expect(screen.queryByRole('cell', { name: '张三' })).not.toBeInTheDocument())
    expect(screen.getByText('没有符合当前筛选条件的申请')).toBeVisible()
  }, 10_000)

  it('用户管理提供新增、权限详情和全生命周期操作', async () => {
    vi.mocked(getUsers).mockResolvedValue([{ accountId: 'user-1', loginName: '24010001', studentNo: '24010001', name: '张三', status: 'ACTIVE', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false, departmentId: '', organizationRoleId: '' }])
    renderPage(<UserManagementPage />)
    expect(await screen.findByRole('button', { name: '新增用户' })).toBeInTheDocument()
    await screen.findByRole('cell', { name: '张三' })
    expect(screen.getByRole('button', { name: '权限详情' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '重置密码' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '禁用' })).toBeInTheDocument()
  }, 10_000)

  it('用户表格使用 ProTable 原生查询区并支持关键字筛选', async () => {
    vi.mocked(getUsers).mockResolvedValue([
      { accountId: 'user-1', loginName: '24010001', studentNo: '24010001', name: '张三', status: 'ACTIVE', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false, departmentId: 'department-1', organizationRoleId: 'role-1' },
    ])
    const { container } = renderPage(<UserManagementPage />)

    await screen.findByRole('cell', { name: '张三' })
    expect(container.querySelector('.ant-pro-query-filter')).not.toBeNull()
    expect(container.querySelector('.user-filter-panel')).toBeNull()
    fireEvent.change(screen.getByPlaceholderText('输入账号、学号或姓名…'), { target: { value: '李四' } })
    fireEvent.click(screen.getByRole('button', { name: /查.*询/ }))

    await waitFor(() => expect(screen.queryByRole('cell', { name: '张三' })).not.toBeInTheDocument())
    expect(screen.getByText('共 0 名用户')).toBeVisible()
    fireEvent.click(screen.getByText('展开'))
    expect(screen.getByLabelText('系统角色')).toBeVisible()
    expect(screen.getByLabelText('账号状态')).toBeVisible()
    expect(screen.getByLabelText('所属部门')).toBeVisible()
    expect(screen.getByLabelText('组织角色')).toBeVisible()
  }, 10_000)

  it('点击权限详情后打开对应用户的权限抽屉', async () => {
	vi.mocked(getUsers).mockResolvedValue([{ accountId: 'user-1', loginName: '24010001', studentNo: '24010001', name: '张三', status: 'ACTIVE', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false, departmentId: '', organizationRoleId: '' }])
	renderPage(<UserManagementPage />)
	fireEvent.click(await screen.findByRole('button', { name: '权限详情' }))
	expect(await screen.findByText('张三 · 生效权限')).toBeVisible()
  })

  it('权限详情使用中文展示系统配置查看权限', async () => {
	vi.mocked(getUsers).mockResolvedValue([{ accountId: 'admin-1', loginName: 'admin', studentNo: '', name: '超级管理员', status: 'ACTIVE', systemRole: 'SUPER_ADMIN', isSuperAdmin: true, mustChangePassword: false, departmentId: '', organizationRoleId: '' }])
	vi.mocked(getEffectivePermissions).mockResolvedValue({ accountId: 'admin-1', systemRole: 'SUPER_ADMIN', permissions: [{ action: 'CONFIGURATION_VIEW', effect: 'ALLOW', scope: {}, sources: [{ type: 'SYSTEM_ROLE', label: '超级管理员' }] }] })
	renderPage(<UserManagementPage />)

	fireEvent.click(await screen.findByRole('button', { name: '权限详情' }))

	expect(await screen.findByText('系统配置查看')).toBeVisible()
	expect(screen.queryByText('CONFIGURATION_VIEW')).not.toBeInTheDocument()
  })

  it('权限详情分别展示同一动作对应的部门和检查类型', async () => {
    vi.mocked(getUsers).mockResolvedValue([{ accountId: 'user-1', loginName: '24010001', studentNo: '24010001', name: '办公室成员', status: 'ACTIVE', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false, departmentId: 'office', organizationRoleId: 'role-1' }])
    vi.mocked(getCatalogs).mockResolvedValue({ departments: [{ id: 'office', code: 'OFFICE', name: '学生会办公室', enabled: true }], organizationRoles: [], inspectionTypes: [{ id: 'attendance', code: 'ATTENDANCE', name: '到课率', extensionKind: '', enabled: true, participatesInReport: true, sortOrder: 1 }, { id: 'hygiene', code: 'HYGIENE', name: '宿舍卫生', extensionKind: '', enabled: true, participatesInReport: true, sortOrder: 2 }] })
    vi.mocked(getEffectivePermissions).mockResolvedValue({ accountId: 'user-1', systemRole: 'MEMBER', permissions: [
      { action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: { departmentId: 'office', inspectionTypeId: 'attendance' }, sources: [{ type: 'DEPARTMENT_MAPPING', label: '部门映射' }] },
      { action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: { departmentId: 'office', inspectionTypeId: 'hygiene' }, sources: [{ type: 'DEPARTMENT_MAPPING', label: '部门映射' }] },
    ] })
    renderPage(<UserManagementPage />)
    fireEvent.click(await screen.findByRole('button', { name: '权限详情' }))
    expect(await screen.findByText('部门：学生会办公室；检查类型：到课率')).toBeInTheDocument()
    expect(screen.getByText('部门：学生会办公室；检查类型：宿舍卫生')).toBeInTheDocument()
  })

  it('重置密码由服务端生成并只在结果弹窗显示', async () => {
    vi.mocked(getUsers).mockResolvedValue([{ accountId: 'user-1', loginName: '24010001', studentNo: '24010001', name: '张三', status: 'ACTIVE', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false, departmentId: '', organizationRoleId: '' }])
    vi.mocked(resetUserPassword).mockResolvedValue({ temporaryPassword: 'generated-password-123456' })
    renderPage(<UserManagementPage />)
    fireEvent.click(await screen.findByRole('button', { name: '重置密码' }))
    expect(screen.queryByLabelText('新临时密码')).not.toBeInTheDocument()
    expect(screen.getByText(/下一弹窗显示/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '确 定' }))
    await waitFor(() => expect(resetUserPassword).toHaveBeenCalledWith('user-1'))
    expect(await screen.findByText('generated-password-123456')).toBeInTheDocument()
    expect(screen.getByText('密码重置成功')).toBeInTheDocument()
    expect(screen.getAllByText('24010001')).toHaveLength(3)
    expect(screen.getByText('初始密码')).toBeInTheDocument()
  })

  it('未分配部门和组织角色使用与系统角色一致的标签样式', async () => {
    vi.mocked(getUsers).mockResolvedValue([{ accountId: 'user-1', loginName: '24010001', studentNo: '24010001', name: '张三', status: 'ACTIVE', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false, departmentId: '', organizationRoleId: '' }])
    renderPage(<UserManagementPage />)
    await screen.findByRole('cell', { name: '张三' })
    const unassigned = screen.getAllByText('未分配')
    expect(unassigned).toHaveLength(2)
    for (const label of unassigned) expect(label).toHaveClass('ant-tag', 'user-assignment-tag')
  })

  it('权限详情把后端 null 集合按空列表展示而不会白屏', async () => {
	vi.mocked(getUsers).mockResolvedValue([{ accountId: 'user-1', loginName: '24010001', studentNo: '24010001', name: '张三', status: 'ACTIVE', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false, departmentId: '', organizationRoleId: '' }])
	vi.mocked(getEffectivePermissions).mockResolvedValue({ accountId: 'user-1', systemRole: 'MEMBER', permissions: null } as never)
	vi.mocked(getPermissionRules).mockResolvedValue(null as never)
	vi.mocked(getMasterData).mockResolvedValue({ items: null, total: 0, page: 1, pageSize: 100 } as never)
	renderPage(<UserManagementPage />)

	fireEvent.click(await screen.findByRole('button', { name: '权限详情' }))

	await waitFor(() => expect(getEffectivePermissions).toHaveBeenCalledWith('user-1'))
	await waitFor(() => expect(getPermissionRules).toHaveBeenCalledWith('user-1'))
	expect(await screen.findByText('当前没有生效权限')).toBeVisible()
	expect(screen.getByText('暂无用户单独规则')).toBeVisible()
  })

  it('用户读取失败不显示正常空态，重试后恢复列表', async () => {
    vi.mocked(getUsers).mockRejectedValueOnce(new Error('request failed')).mockResolvedValue([])
    renderPage(<UserManagementPage />)
    expect(await screen.findByText('用户列表加载失败')).toBeVisible()
    expect(screen.queryByText('暂无用户')).not.toBeInTheDocument()
    expect(screen.queryByText('共 0 名用户')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '重试用户列表' }))
    expect(await screen.findByText('暂无用户')).toBeVisible()
    expect(screen.queryByText('用户列表加载失败')).not.toBeInTheDocument()
  })

  it('组织目录读取失败不误报归属停用，重试后恢复名称', async () => {
    vi.mocked(getUsers).mockResolvedValue([{ accountId: 'user-1', loginName: '24010001', studentNo: '24010001', name: '张三', status: 'ACTIVE', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false, departmentId: 'department-1', organizationRoleId: 'role-1' }])
    vi.mocked(getCatalogs).mockRejectedValueOnce(new Error('request failed')).mockResolvedValue({ departments: [{ id: 'department-1', code: 'ONE', name: '部门一', enabled: true }], organizationRoles: [{ id: 'role-1', code: 'MEMBER', name: '委员', enabled: true, sortOrder: 1 }], inspectionTypes: [] })
    renderPage(<UserManagementPage />)
    expect(await screen.findByText('组织目录加载失败')).toBeVisible()
    expect(screen.queryByText('部门已停用')).not.toBeInTheDocument()
    expect(screen.queryByText('角色已停用')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '重试组织目录' }))
    expect(await screen.findByText('部门一')).toBeVisible()
    expect(screen.queryByText('组织目录加载失败')).not.toBeInTheDocument()
  })

  it('编辑中组织目录刷新失败保留输入，恢复前不提交', async () => {
    vi.mocked(getUsers).mockResolvedValue([{ accountId: 'user-1', loginName: '24010001', studentNo: '24010001', name: '张三', status: 'ACTIVE', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false, departmentId: '', organizationRoleId: '' }])
    const { client } = renderPage(<UserManagementPage />)
    fireEvent.click(await screen.findByRole('button', { name: '编辑' }))
    const dialog = screen.getByRole('dialog')
    fireEvent.change(within(dialog).getByLabelText('姓名'), { target: { value: '未保存姓名' } })
    vi.mocked(getCatalogs).mockRejectedValue(new Error('request failed'))
    await client.invalidateQueries({ queryKey: ['admin-catalogs'] })
    await waitFor(() => expect(within(dialog).getByText('组织目录加载失败，请重试后保存')).toBeVisible())
    expect(within(dialog).getByLabelText('姓名')).toHaveValue('未保存姓名')
    expect(within(dialog).getByRole('button', { name: '确 定' })).toBeDisabled()
    const form = dialog.querySelector('form')
    if (!form) throw new Error('Editor form missing')
    const saveError = vi.spyOn(message, 'error')
    fireEvent.submit(form)
    await waitFor(() => expect(saveError).toHaveBeenCalledWith('组织目录加载失败，请重试后保存'))
    saveError.mockRestore()
    expect(updateUser).not.toHaveBeenCalled()
    vi.mocked(getCatalogs).mockResolvedValue({ departments: [], organizationRoles: [], inspectionTypes: [] })
    fireEvent.click(within(dialog).getByRole('button', { name: '重试组织目录' }))
    await waitFor(() => expect(within(dialog).getByRole('button', { name: '确 定' })).not.toBeDisabled())
    expect(within(dialog).getByLabelText('姓名')).toHaveValue('未保存姓名')
  })

  it('未分配账号可只修改姓名，失败保留输入并原位重试', async () => {
    vi.mocked(getUsers).mockResolvedValue([{ accountId: 'user-1', loginName: '24010001', studentNo: '24010001', name: '张三', status: 'ACTIVE', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false, departmentId: '', organizationRoleId: '' }])
    vi.mocked(updateUser).mockRejectedValueOnce(new Error('账号状态已变化，请刷新后重试')).mockResolvedValue({ id: 'user-1' } as never)
    renderPage(<UserManagementPage />)
    fireEvent.click(await screen.findByRole('button', { name: '编辑' }))
    const dialog = screen.getByRole('dialog')
    fireEvent.change(within(dialog).getByLabelText('姓名'), { target: { value: '保留的新姓名' } })
    fireEvent.click(within(dialog).getByRole('button', { name: '确 定' }))
    await waitFor(() => expect(updateUser).toHaveBeenCalledTimes(1))
    await screen.findByText('账号状态已变化，请刷新后重试')
    expect(within(dialog).getByLabelText('姓名')).toHaveValue('保留的新姓名')
    expect(assignUserRole).not.toHaveBeenCalled()
    fireEvent.click(within(dialog).getByRole('button', { name: '确 定' }))
    await waitFor(() => expect(updateUser).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(assignUserRole).not.toHaveBeenCalled()
  })

  it('编辑表单回显归属，并在保存时提交部门和角色', async () => {
    vi.mocked(getUsers).mockResolvedValue([{ accountId: 'user-1', loginName: '24010001', studentNo: '24010001', name: '张三', status: 'ACTIVE', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false, departmentId: 'department-1', organizationRoleId: 'role-1' }])
    vi.mocked(getCatalogs).mockResolvedValue({ departments: [{ id: 'department-1', code: 'ONE', name: '部门一', enabled: true }, { id: 'department-2', code: 'TWO', name: '部门二', enabled: true }], organizationRoles: [{ id: 'role-1', code: 'MEMBER', name: '委员', enabled: true, sortOrder: 1 }], inspectionTypes: [] })
    vi.mocked(assignUserRole).mockResolvedValue(undefined)
    vi.mocked(updateUser).mockResolvedValue({ id: 'user-1' } as never)
    renderPage(<UserManagementPage />)

    fireEvent.click(await screen.findByRole('button', { name: '编辑' }))
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByDisplayValue('24010001')).toBeInTheDocument()
    expect(within(dialog).getByDisplayValue('张三')).toBeInTheDocument()
    expect(within(dialog).getByText('部门一')).toBeInTheDocument()
    expect(within(dialog).getByText('委员')).toBeInTheDocument()
    for (const control of within(dialog).getAllByRole('combobox')) {
      expect(document.querySelectorAll(`[id="${control.id}"]`)).toHaveLength(1)
    }
    expect(within(dialog).getByRole('combobox', { name: '系统角色' })).toBeInTheDocument()
    expect(within(dialog).getByRole('combobox', { name: '组织角色' })).toBeInTheDocument()
    fireEvent.mouseDown(within(dialog).getByRole('combobox', { name: '所属部门' }))
    fireEvent.click(await screen.findByText('部门二'))
    fireEvent.click(within(dialog).getByRole('button', { name: '确 定' }))

    await waitFor(() => expect(assignUserRole).toHaveBeenCalled())
    expect(assignUserRole).toHaveBeenCalledWith(expect.objectContaining({ departmentId: 'department-2', organizationRoleId: 'role-1' }))
  })

  it('基础数据页面使用分类导航和 ProTable 新增入口', async () => {
    renderPage(<MasterDataPage />)
    expect(await screen.findByRole('menuitem', { name: '学生' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: '班级' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新增学生' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '导入Excel' })).toBeInTheDocument()
  })

  it('基础数据 Excel 导入按分类展示字段、预览并确认整批新增', async () => {
    const spec: MasterDataImportSpec = {
      kind: 'faculties', templateCode: 'MASTER_DATA_FACULTIES', templateVersion: 1, sheetName: '院系导入', label: '院系',
      fields: [{ code: 'name', header: '院系名称', required: true, maxRunes: 96, format: '必填，1–96 个字符' }],
    }
    const batch: MasterDataImportBatch = {
      id: 'batch-1', kind: 'faculties', status: 'PREVIEW_READY', templateVersion: 1, fields: spec.fields,
      totalRows: 1, validRows: 1, invalidRows: 0, version: 1, createdAt: '2026-10-24T00:00:00Z',
      rows: [{ rowNo: 5, values: { name: '测试学院' }, status: 'VALID', errors: [], record: { id: 'faculty-1', kind: 'faculties', name: '测试学院', enabled: true } }],
    }
    vi.mocked(getMasterDataImportSpec).mockResolvedValue(spec)
    vi.mocked(downloadMasterDataImportTemplate).mockResolvedValue({ blob: new Blob(), headers: new Headers() })
    vi.mocked(previewMasterDataImport).mockResolvedValue(batch)
    vi.mocked(confirmMasterDataImportBatch).mockResolvedValue({ batchId: 'batch-1', kind: 'faculties', status: 'CONFIRMED', importedRows: 1, version: 2 })
    renderPage(<MasterDataPage />)

    fireEvent.click(await screen.findByRole('menuitem', { name: '院系' }))
    fireEvent.click(await screen.findByRole('button', { name: '导入Excel' }))
    const dialog = await screen.findByRole('dialog', { name: '导入院系' })
    expect(await within(dialog).findByText(/院系名称（必填）/)).toBeInTheDocument()
    fireEvent.click(within(dialog).getByRole('button', { name: '下载当前分类模板' }))
    await waitFor(() => expect(downloadMasterDataImportTemplate).toHaveBeenCalledWith('faculties'))
    const input = within(dialog).getByLabelText('选择基础数据导入文件')
    fireEvent.change(input, { target: { files: [new File(['workbook'], 'faculties.xlsx', { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })] } })
    fireEvent.click(within(dialog).getByRole('button', { name: '上传并预览' }))

    expect(await within(dialog).findByText('测试学院')).toBeVisible()
    expect(previewMasterDataImport).toHaveBeenCalledWith('faculties', expect.objectContaining({ name: 'faculties.xlsx' }))
    fireEvent.click(within(dialog).getByRole('button', { name: '确认新增' }))
    fireEvent.click(await screen.findByRole('button', { name: '确认导入' }))
    await waitFor(() => expect(confirmMasterDataImportBatch).toHaveBeenCalledWith('faculties', 'batch-1', 1, expect.any(String)))
    expect(await screen.findByText('成功新增 1 条院系数据')).toBeVisible()
  }, 15_000)

  it('课程分类不显示基础数据 Excel 导入入口', async () => {
    renderPage(<MasterDataPage />)
    fireEvent.click(await screen.findByRole('menuitem', { name: '课程' }))
    await waitFor(() => expect(screen.getByRole('button', { name: '新增课程' })).toBeVisible())
    expect(screen.queryByRole('button', { name: '导入Excel' })).not.toBeInTheDocument()
  })

  it('基础数据 Excel 预览有错误时不提供确认新增入口', async () => {
    vi.mocked(previewMasterDataImport).mockResolvedValue({
      id: 'batch-with-errors', kind: 'faculties', status: 'PREVIEW_HAS_ERRORS', templateVersion: 1,
      fields: [{ code: 'name', header: '院系名称', required: true, format: '必填' }], totalRows: 2, validRows: 1,
      invalidRows: 1, version: 1, createdAt: '2026-10-24T00:00:00Z', rows: [
        { rowNo: 5, values: { name: '可新增学院' }, status: 'VALID', errors: [] },
        { rowNo: 6, values: { name: '重复学院' }, status: 'INVALID', errors: [{ field: 'name', message: '与已有基础数据重复' }] },
      ],
    })
    renderPage(<MasterDataPage />)
    fireEvent.click(await screen.findByRole('menuitem', { name: '院系' }))
    fireEvent.click(await screen.findByRole('button', { name: '导入Excel' }))
    const dialog = await screen.findByRole('dialog', { name: '导入院系' })
    fireEvent.change(within(dialog).getByLabelText('选择基础数据导入文件'), {
      target: { files: [new File(['workbook'], 'faculties.xlsx')] },
    })
    fireEvent.click(within(dialog).getByRole('button', { name: '上传并预览' }))
    expect(await within(dialog).findByText('预览发现错误，整批不能导入')).toBeInTheDocument()
    expect(within(dialog).getByText(/与已有基础数据重复/)).toBeInTheDocument()
    expect(within(dialog).queryByRole('button', { name: '确认新增' })).not.toBeInTheDocument()
    expect(confirmMasterDataImportBatch).not.toHaveBeenCalled()
  }, 15_000)

  it('基础数据按业务分组在左侧导航并在右侧展示表格', async () => {
    const { container } = renderPage(<MasterDataPage />)
    expect(await screen.findByText('基础信息')).toBeVisible()
    expect(screen.getByText('宿舍相关')).toBeVisible()
    expect(screen.getByText('教学相关')).toBeVisible()
    expect(screen.getByText('宿舍区')).toBeVisible()
    expect(screen.getByText('宿舍楼')).toBeVisible()
    expect(container.querySelector('.master-data-workspace')).not.toBeNull()
    expect(container.querySelector('.master-data-navigation')).not.toBeNull()
  })

  it('基础数据查询区分别提供编码和名称且不再显示合并筛选', async () => {
    renderPage(<MasterDataPage />)
    expect(await screen.findByLabelText('学号')).toBeVisible()
    expect(screen.getByLabelText('名称')).toBeVisible()
    expect(screen.queryByLabelText('编码或名称')).not.toBeInTheDocument()
  })

  it('基础数据只加载当前页签及其表单依赖，切换页签时加载新页签', async () => {
    renderPage(<MasterDataPage />)
    await waitFor(() => expect(getMasterData).toHaveBeenCalledTimes(8))
    expect(new Set(vi.mocked(getMasterData).mock.calls.map(call => call[0]))).toEqual(new Set(['students', 'grades', 'faculties', 'majors', 'classes', 'dormitory-areas', 'dormitory-buildings', 'dormitories']))

    vi.mocked(getMasterData).mockClear()
    fireEvent.click(screen.getByRole('menuitem', { name: '院系' }))
    await waitFor(() => expect(getMasterData).toHaveBeenCalledWith('faculties', expect.objectContaining({ page: 1, pageSize: 20 })))
    expect(vi.mocked(getMasterData).mock.calls.every(call => call[0] === 'faculties')).toBe(true)
  })

  it('基础数据关联下拉遍历所有分页结果', async () => {
    vi.mocked(getMasterData).mockImplementation(async (kind, params) => {
      if (kind === 'grades' && params?.enabled && params.page === 1) {
        return {
          items: Array.from({ length: 100 }, (_, index) => ({ id: `grade-${index}`, kind, name: `${1900 + index}级`, enabled: true })),
          total: 101,
          page: 1,
          pageSize: 100,
        }
      }
      if (kind === 'grades' && params?.enabled && params.page === 2) {
        return { items: [{ id: 'grade-last', kind, name: '2000级', enabled: true }], total: 101, page: 2, pageSize: 100 }
      }
      return { items: [], total: 0, page: params?.page ?? 1, pageSize: params?.pageSize ?? 20 }
    })

    renderPage(<MasterDataPage />)

    await waitFor(() => expect(getMasterData).toHaveBeenCalledWith('grades', expect.objectContaining({ enabled: true, page: 2, pageSize: 100 })))
  })

  it('基础数据 ProTable 展示当前类型的名称、状态和关系筛选', async () => {
    renderPage(<MasterDataPage />)
    expect(await screen.findByLabelText('学号')).toBeVisible()
    expect(screen.getByLabelText('名称')).toBeVisible()
    expect(screen.getByLabelText('状态')).toBeVisible()
    expect(screen.getByLabelText('所属院系')).toBeVisible()
    expect(screen.getByLabelText('专业')).toBeVisible()

    fireEvent.click(screen.getByRole('menuitem', { name: '专业' }))
    expect(await screen.findByLabelText('名称')).toBeVisible()
    expect(screen.queryByLabelText('专业编码')).not.toBeInTheDocument()
    expect(screen.getByLabelText('所属院系')).toBeVisible()
  })

  it('专业和学生表格展示完整的业务关联字段', async () => {
    vi.mocked(getMasterData).mockImplementation(async kind => {
      const records: Partial<Record<MasterDataKind, MasterDataRecord[]>> = {
        faculties: [{ id: 'faculty-1', kind: 'faculties', name: '计算机学院', enabled: true }],
        majors: [{ id: 'major-1', kind: 'majors', name: '软件工程', facultyId: 'faculty-1', enabled: true }],
        grades: [{ id: 'grade-1', kind: 'grades', name: '2026级', enabled: true }],
        classes: [{ id: 'class-1', kind: 'classes', name: '软件工程一班', gradeId: 'grade-1', grade: '2026级', facultyId: 'faculty-1', majorId: 'major-1', enabled: true }],
        students: [{ id: 'student-1', kind: 'students', code: '20260001', name: '张三', gradeId: 'grade-1', grade: '2026级', facultyId: 'faculty-1', majorId: 'major-1', classId: 'class-1', enabled: true }],
      }
      const items = records[kind] ?? []
      return { items, total: items.length, page: 1, pageSize: 20 }
    })
    renderPage(<MasterDataPage />)

    expect(await screen.findByRole('cell', { name: '计算机学院' })).toBeVisible()
    expect(screen.getByRole('cell', { name: '软件工程' })).toBeVisible()
    expect(screen.getByRole('cell', { name: '软件工程一班' })).toBeVisible()
    expect(screen.getByRole('cell', { name: '2026级' })).toBeVisible()

    fireEvent.click(screen.getByRole('menuitem', { name: '专业' }))
    expect(await screen.findByRole('cell', { name: '计算机学院' })).toBeVisible()
  })

  it('宿舍和宿舍楼使用完整层级名称并提示宿舍号示例', async () => {
    vi.mocked(getMasterData).mockImplementation(async kind => {
      const records: Partial<Record<MasterDataKind, MasterDataRecord[]>> = {
        'dormitory-areas': [{ id: 'area-4', kind: 'dormitory-areas', name: '4 区', enabled: true }],
        'dormitory-buildings': [{ id: 'building-1', kind: 'dormitory-buildings', name: '1 栋', dormitoryAreaId: 'area-4', enabled: true }],
        dormitories: [{ id: 'room-401', kind: 'dormitories', name: '401', roomNo: '401', dormitoryAreaId: 'area-4', dormitoryBuildingId: 'building-1', enabled: true }],
      }
      const items = records[kind] ?? []
      return { items, total: items.length, page: 1, pageSize: 20 }
    })
    renderPage(<MasterDataPage />)

    fireEvent.click(await screen.findByRole('menuitem', { name: '宿舍楼' }))
    expect(await screen.findByRole('cell', { name: '4 区 1 栋' })).toBeVisible()

    fireEvent.click(screen.getByRole('menuitem', { name: '宿舍' }))
    expect(await screen.findByRole('cell', { name: '4 区 1 栋 401' })).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '新增宿舍' }))
    const dialog = await screen.findByRole('dialog', { name: '新增宿舍' })
    expect(within(dialog).getByText('例如：401、402')).toBeInTheDocument()
    expect(within(dialog).getByPlaceholderText('例如：401')).toBeInTheDocument()
  })

  it('教学楼独立维护且教室选择教学楼而不再填写楼层', async () => {
    vi.mocked(getMasterData).mockImplementation(async kind => {
      const records: Partial<Record<MasterDataKind, MasterDataRecord[]>> = {
        'teaching-buildings': [{ id: 'teaching-building-1', kind: 'teaching-buildings', name: '博学楼', enabled: true }],
        classrooms: [{ id: 'classroom-101', kind: 'classrooms', name: '博学楼 101', teachingBuildingId: 'teaching-building-1', roomNo: '101', enabled: true }],
      }
      const items = records[kind] ?? []
      return { items, total: items.length, page: 1, pageSize: 20 }
    })
    renderPage(<MasterDataPage />)

    fireEvent.click(await screen.findByRole('menuitem', { name: '教学楼' }))
    expect(await screen.findByRole('cell', { name: '博学楼' })).toBeVisible()

    fireEvent.click(screen.getByRole('menuitem', { name: '教室' }))
    expect(await screen.findByRole('cell', { name: '博学楼 101' })).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '新增教室' }))
    const dialog = await screen.findByRole('dialog', { name: '新增教室' })
    expect(within(dialog).getByLabelText('教学楼')).toHaveAttribute('role', 'combobox')
    expect(within(dialog).getByLabelText('教室号')).toBeInTheDocument()
    expect(within(dialog).queryByLabelText('楼层')).not.toBeInTheDocument()
  })

  it('院系、专业、班级和课程不再要求管理员维护编码', async () => {
    renderPage(<MasterDataPage />)

    fireEvent.click(await screen.findByRole('menuitem', { name: '院系' }))
    expect(await screen.findByLabelText('名称')).toBeVisible()
    expect(screen.queryByLabelText('院系编码')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '新增院系' }))
    expect(within(await screen.findByRole('dialog', { name: '新增院系' })).queryByLabelText('院系编码')).not.toBeInTheDocument()
  })

  it('课程支持多班级和学期星期节次复合排课', async () => {
    vi.mocked(getAcademicCalendar).mockResolvedValue({
      academicYears: [],
      semesters: [{ id: 'semester-1', academicYearId: 'year-1', code: 'FIRST', name: '2026—2027 学年第一学期', startDate: '2026-09-01T00:00:00Z', endDate: '2027-01-15T00:00:00Z', enabled: true }],
      weeks: [],
    })
    renderPage(<MasterDataPage />)

    fireEvent.click(await screen.findByRole('menuitem', { name: '课程' }))
    fireEvent.click(await screen.findByRole('button', { name: '新增课程' }))
    const dialog = await screen.findByRole('dialog', { name: '新增课程' })
    expect(within(dialog).queryByLabelText('课程编码')).not.toBeInTheDocument()
    expect(within(dialog).getByLabelText('授课班级')).toHaveAttribute('role', 'combobox')
    expect(within(dialog).getByLabelText('所属学期')).toHaveAttribute('role', 'combobox')
    expect(within(dialog).getByLabelText('星期')).toHaveAttribute('role', 'combobox')
    expect(within(dialog).getByLabelText('开始节次')).toHaveAttribute('aria-valuemin', '1')
    expect(within(dialog).getByLabelText('结束节次')).toHaveAttribute('aria-valuemax', '10')
    expect(within(dialog).getByText('45 分钟/节')).toBeInTheDocument()
  })

  it('年级可独立维护且学生表单从年级基础数据中选择', async () => {
    vi.mocked(getMasterData).mockImplementation(async kind => ({
      items: kind === 'grades' ? [{ id: 'grade-2026', kind: 'grades', name: '2026级', enabled: true }] : [],
      total: kind === 'grades' ? 1 : 0, page: 1, pageSize: 20,
    }))
    renderPage(<MasterDataPage />)
    expect(await screen.findByRole('menuitem', { name: '年级' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '新增学生' }))
    const dialog = await screen.findByRole('dialog', { name: '新增学生' })
    const gradeInputId = dialog.querySelector('label[title="年级"]')?.getAttribute('for')
    expect(gradeInputId).toBeTruthy()
    expect(document.getElementById(gradeInputId!)).toHaveAttribute('role', 'combobox')
    expect(getMasterData).toHaveBeenCalledWith('grades', expect.objectContaining({ enabled: true }))
  })

  it('基础数据关联下拉选项只显示完整名称', async () => {
    vi.mocked(getMasterData).mockImplementation(async kind => ({
      items: kind === 'faculties' ? [{ id: 'faculty-1', kind, code: 'SCHOOL_OF_COMPUTER_SCIENCE_AND_ENGINEERING', name: '计算机科学与工程学院', enabled: true }] : [],
      total: kind === 'faculties' ? 1 : 0, page: 1, pageSize: 20,
    }))
    renderPage(<MasterDataPage />)

    fireEvent.click(await screen.findByRole('button', { name: '新增学生' }))
    const dialog = await screen.findByRole('dialog', { name: '新增学生' })
    fireEvent.mouseDown(within(dialog).getByLabelText('所属院系'))

    expect(await screen.findByRole('option', { name: '计算机科学与工程学院' })).toBeInTheDocument()
    expect(screen.queryByText(/SCHOOL_OF_COMPUTER_SCIENCE_AND_ENGINEERING/)).not.toBeInTheDocument()
  })

  it('年级只录入年份而不暴露内部编码', async () => {
    vi.mocked(createMasterData).mockResolvedValue({ id: 'grade-2027', kind: 'grades', code: '2027', name: '2027级', enabled: true })
    renderPage(<MasterDataPage />)

    fireEvent.click(await screen.findByRole('menuitem', { name: '年级' }))
    expect(await screen.findByLabelText('年级')).toBeVisible()
    expect(screen.queryByLabelText('年级编码')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('名称')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '新增年级' }))
    const dialog = await screen.findByRole('dialog', { name: '新增年级' })
    expect(within(dialog).queryByLabelText('年级编码')).not.toBeInTheDocument()
    fireEvent.change(within(dialog).getByLabelText('年级'), { target: { value: '2027' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /确\s*定/ }))

    await waitFor(() => expect(createMasterData).toHaveBeenCalledWith('grades', { name: '2027' }))
  })

  it('宿舍楼依赖宿舍区且宿舍通过选择项维护层级', async () => {
    vi.mocked(getMasterData).mockImplementation(async kind => ({
      items: kind === 'dormitory-areas'
        ? [{ id: 'area-north', kind, code: 'NORTH', name: '北校区', enabled: true }]
        : kind === 'dormitory-buildings'
          ? [{ id: 'building-1', kind, code: '1', name: '一栋', enabled: true, dormitoryAreaId: 'area-north' }]
          : [],
      total: 0, page: 1, pageSize: 20,
    }))
    renderPage(<MasterDataPage />)

    fireEvent.click(await screen.findByRole('menuitem', { name: '宿舍楼' }))
    fireEvent.click(await screen.findByRole('button', { name: '新增宿舍楼' }))
    let dialog = await screen.findByRole('dialog', { name: '新增宿舍楼' })
    expect(within(dialog).getByLabelText('所属宿舍区')).toHaveAttribute('role', 'combobox')
    fireEvent.click(within(dialog).getByRole('button', { name: /取\s*消/ }))

    fireEvent.click(screen.getByRole('menuitem', { name: '宿舍' }))
    fireEvent.click(await screen.findByRole('button', { name: '新增宿舍' }))
    dialog = await screen.findByRole('dialog', { name: '新增宿舍' })
    expect(within(dialog).getByLabelText('宿舍区')).toHaveAttribute('role', 'combobox')
    expect(within(dialog).getByLabelText('宿舍楼')).toHaveAttribute('role', 'combobox')
  }, 15_000)

  it('基础数据编辑弹窗回显当前记录', async () => {
    vi.mocked(getMasterData).mockImplementation(async kind => ({
      items: kind === 'faculties' ? [{
        id: 'faculty-1', kind: 'faculties',
        name: '计算机科学与工程学院', enabled: true,
      }] : [],
      total: kind === 'faculties' ? 1 : 0,
      page: 1,
      pageSize: 20,
    }))
    renderPage(<MasterDataPage />)

    fireEvent.click(await screen.findByRole('menuitem', { name: '院系' }))
    await screen.findByRole('cell', { name: '计算机科学与工程学院' })
    fireEvent.click(screen.getByRole('button', { name: '编辑' }))

    const dialog = await screen.findByRole('dialog', { name: '编辑院系' })
    expect(within(dialog).queryByRole('textbox', { name: '院系编码' })).not.toBeInTheDocument()
    expect(within(dialog).getByRole('textbox', { name: '名称' })).toHaveValue('计算机科学与工程学院')
  }, 10_000)

  it('基础数据页面把后端 null 集合按空列表展示而不会白屏', async () => {
	vi.mocked(getMasterData).mockResolvedValue({ items: null, total: 0, page: 1, pageSize: 20 } as never)
	vi.mocked(getDictionaries).mockResolvedValue({ types: null, items: null } as never)
	renderPage(<MasterDataPage />)

	await waitFor(() => expect(getMasterData).toHaveBeenCalledTimes(8))
	await waitFor(() => expect(getDictionaries).toHaveBeenCalled())
	expect(await screen.findByText('暂无学生数据')).toBeVisible()
	expect(screen.getByRole('button', { name: '新增学生' })).toBeVisible()
  })

  it('学生基础数据表单使用系统字典选项', async () => {
    vi.mocked(getDictionaries).mockImplementation(async typeId => {
      if (!typeId) return { types: [
        { id: 'gender-type', code: 'GENDER', name: '性别', description: '', system: true, enabled: true },
        { id: 'status-type', code: 'ENROLLMENT_STATUS', name: '学籍状态', description: '', system: true, enabled: true },
      ], items: [] }
      if (typeId === 'gender-type') return { types: [], items: [{ id: 'gender-male', typeId, value: 'MALE', label: '男（系统字典）', description: '', sortOrder: 10, enabled: true }] }
      return { types: [], items: [{ id: 'status-enrolled', typeId, value: 'ENROLLED', label: '在籍（系统字典）', description: '', sortOrder: 10, enabled: true }] }
    })

    renderPage(<MasterDataPage />)
    fireEvent.click(await screen.findByRole('button', { name: '新增学生' }))
    const dialog = await screen.findByRole('dialog', { name: '新增学生' })
    fireEvent.mouseDown(within(dialog).getByLabelText('性别'))
    expect(await screen.findByText('男（系统字典）')).toBeInTheDocument()
    fireEvent.mouseDown(within(dialog).getByLabelText('学籍状态'))
    expect((await screen.findAllByText('在籍（系统字典）')).length).toBeGreaterThan(0)
  })

  it('字典页面提供类型与选项的配置入口', async () => {
    renderPage(<DictionaryPage />)
    expect(await screen.findByRole('button', { name: '新增字典类型' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '新增字典类型' }))
    expect(await screen.findByRole('dialog', { name: '新增字典类型' })).toBeInTheDocument()
    expect(screen.getByLabelText('类型编码')).toBeInTheDocument()
  })

	it('字典页面默认选中第一个类型并展示其选项', async () => {
		vi.mocked(getDictionaries).mockImplementation(async typeId => typeId
			? { types: [], items: [{ id: 'item-1', typeId, value: 'ENROLLED', label: '在籍', description: '', sortOrder: 10, enabled: true }] }
			: { types: [
				{ id: 'type-1', code: 'STATUS', name: '学籍状态', description: '', system: false, enabled: true },
				{ id: 'type-2', code: 'GENDER', name: '性别', description: '', system: true, enabled: true },
			], items: [] })
		renderPage(<DictionaryPage />)

		await waitFor(() => expect(getDictionaries).toHaveBeenCalledWith('type-1'))
		expect(screen.getAllByRole('radio')[0]).toBeChecked()
		expect(await screen.findByText('字典选项 · 学籍状态')).toBeVisible()
		expect(await screen.findByRole('cell', { name: '在籍' })).toBeVisible()
	}, 10_000)

	it('字典类型的选中状态不提供取消选择操作', async () => {
		vi.mocked(getDictionaries).mockResolvedValue({
			types: [{ id: 'type-1', code: 'STATUS', name: '学籍状态', description: '', system: false, enabled: true }],
			items: [],
		})
		renderPage(<DictionaryPage />)

		fireEvent.click(await screen.findByRole('radio'))
		expect(screen.queryByText('取消选择')).not.toBeInTheDocument()
	})

	it('字典类型表格在窄容器内使用内部横向滚动', async () => {
		vi.mocked(getDictionaries).mockResolvedValue({
			types: [{ id: 'type-1', code: 'A_VERY_LONG_DICTIONARY_TYPE_CODE', name: '很长的字典类型名称', description: '', system: false, enabled: true }],
			items: [],
		})
		renderPage(<DictionaryPage />)

		const typeCell = await screen.findByRole('cell', { name: 'A_VERY_LONG_DICTIONARY_TYPE_CODE' })
		expect(typeCell.closest('table')).toHaveStyle({ width: 'max-content' })
	})

	it('编辑字典类型时回显原有信息', async () => {
		vi.mocked(getDictionaries).mockResolvedValue({
			types: [{ id: 'type-1', code: 'STATUS', name: '学籍状态', description: '学生当前学籍状态', system: false, enabled: true }],
			items: [],
		})
		renderPage(<DictionaryPage />)

		const typeRow = (await screen.findByRole('cell', { name: '学籍状态' })).closest('tr')
		if (!typeRow) throw new Error('未找到字典类型行')
		fireEvent.click(within(typeRow).getByRole('button', { name: '编辑' }))

		const dialog = await screen.findByRole('dialog', { name: '编辑字典类型' })
		expect(within(dialog).getByLabelText('类型编码')).toHaveValue('STATUS')
		expect(within(dialog).getByLabelText('类型名称')).toHaveValue('学籍状态')
		expect(within(dialog).getByLabelText('说明')).toHaveValue('学生当前学籍状态')
	})

	it('编辑字典项时回显原有信息', async () => {
		vi.mocked(getDictionaries).mockImplementation(async typeId => typeId
			? { types: [], items: [{ id: 'item-1', typeId, value: 'LEFT', label: '离校', description: '已离开学校', sortOrder: 40, enabled: true }] }
			: { types: [{ id: 'type-1', code: 'STATUS', name: '学籍状态', description: '', system: false, enabled: true }], items: [] })
		renderPage(<DictionaryPage />)

		await waitFor(() => expect(getDictionaries).toHaveBeenCalledWith('type-1'))
		fireEvent.click(await screen.findByRole('button', { name: '新增选项' }))
		const createDialog = await screen.findByRole('dialog', { name: '新增字典项' })
		fireEvent.click(within(createDialog).getByRole('button', { name: /取\s*消/ }))
		await waitFor(() => expect(screen.queryByRole('dialog', { name: '新增字典项' })).not.toBeInTheDocument())
		const itemRow = (await screen.findByRole('cell', { name: '离校' })).closest('tr')
		if (!itemRow) throw new Error('未找到字典项行')
		fireEvent.click(within(itemRow).getByRole('button', { name: '编辑' }))

		const dialog = await screen.findByRole('dialog', { name: '编辑字典项' })
		expect(within(dialog).getByLabelText('选项值')).toHaveValue('LEFT')
		expect(within(dialog).getByLabelText('显示名称')).toHaveValue('离校')
		expect(within(dialog).getByLabelText('排序')).toHaveValue('40')
		expect(within(dialog).getByLabelText('说明')).toHaveValue('已离开学校')
		expect(within(dialog).getByRole('switch', { name: '状态' })).toBeChecked()
	}, 10_000)

	it('停用部门前展示对象名称和影响范围并等待确认', async () => {
		vi.mocked(getCatalogs).mockResolvedValue({
			departments: [{ id: 'department-1', code: 'STUDY', name: '学习部', enabled: true }],
			organizationRoles: [], inspectionTypes: [],
		})
		vi.mocked(updateDepartment).mockImplementation(async input => input)
		renderPage(<OrganizationConfigPage />)

		fireEvent.click(await screen.findByRole('switch', { name: '停用部门 学习部' }))
		expect(updateDepartment).not.toHaveBeenCalled()
		const dialog = await screen.findByRole('dialog', { name: /停用“学习部”部门/ })
		expect(within(dialog).getByText(/相关人员归属和业务权限/)).toBeInTheDocument()
		fireEvent.click(within(dialog).getByRole('button', { name: '确认停用' }))
		await waitFor(() => expect(vi.mocked(updateDepartment).mock.calls[0]?.[0]).toEqual(expect.objectContaining({ id: 'department-1', enabled: false })))
	})

	it('停用检查类型前展示通报影响并等待确认', async () => {
		vi.mocked(getCatalogs).mockResolvedValue({
			departments: [], organizationRoles: [],
			inspectionTypes: [{ id: 'inspection-1', code: 'LATE', name: '晚归检查', extensionKind: '', enabled: true, sortOrder: 1, participatesInReport: true }],
		})
		vi.mocked(updateInspectionType).mockImplementation(async input => input)
		renderPage(<OrganizationConfigPage />)

		fireEvent.click(await screen.findByRole('tab', { name: '七类检查' }))
		fireEvent.click(await screen.findByRole('switch', { name: '停用检查类型 晚归检查' }))
		expect(updateInspectionType).not.toHaveBeenCalled()
		const dialog = await screen.findByRole('dialog', { name: /停用“晚归检查”检查类型/ })
		expect(within(dialog).getByText(/录入、审核和通报流程/)).toBeInTheDocument()
		fireEvent.click(within(dialog).getByRole('button', { name: '确认停用' }))
		await waitFor(() => expect(vi.mocked(updateInspectionType).mock.calls[0]?.[0]).toEqual(expect.objectContaining({ id: 'inspection-1', enabled: false })))
	})

	it('修改部门检查权限前展示映射对象和权限影响并等待确认', async () => {
		vi.mocked(getCatalogs).mockResolvedValue({
			departments: [{ id: 'department-1', code: 'STUDY', name: '学习部', enabled: true }], organizationRoles: [],
			inspectionTypes: [{ id: 'inspection-1', code: 'LATE', name: '晚归检查', extensionKind: '', enabled: true, sortOrder: 1, participatesInReport: true }],
		})
		vi.mocked(getDepartmentInspectionScopes).mockResolvedValue([{ departmentId: 'department-1', inspectionTypeId: 'inspection-1', canEnter: false, canView: false, canReview: false, canExport: false, canSummarize: false, enabled: true }])
		vi.mocked(updateDepartmentInspectionScope).mockResolvedValue(undefined)
		renderPage(<OrganizationConfigPage />)

		fireEvent.click(await screen.findByRole('tab', { name: '部门业务映射' }))
		fireEvent.click(await screen.findByRole('switch', { name: '学习部 晚归检查 录入权限' }))
		expect(updateDepartmentInspectionScope).not.toHaveBeenCalled()
		const dialog = await screen.findByRole('dialog', { name: /学习部.*晚归检查.*录入权限/ })
		expect(within(dialog).getByText(/改变该部门成员可执行的业务范围/)).toBeInTheDocument()
		fireEvent.click(within(dialog).getByRole('button', { name: '确认修改' }))
		await waitFor(() => expect(vi.mocked(updateDepartmentInspectionScope).mock.calls[0]?.[0]).toEqual(expect.objectContaining({ canEnter: true })))
	})

	it('停用字典值前展示引用影响并等待确认', async () => {
		vi.mocked(getDictionaries).mockImplementation(async typeId => typeId
			? { types: [], items: [{ id: 'item-1', typeId, value: 'ENROLLED', label: '在籍', description: '', sortOrder: 10, enabled: true }] }
			: { types: [{ id: 'type-1', code: 'STATUS', name: '学籍状态', description: '', system: false, enabled: true }], items: [] })
		vi.mocked(updateDictionaryItem).mockImplementation(async input => input)
		renderPage(<DictionaryPage />)

		fireEvent.click(await screen.findByRole('radio'))
		fireEvent.click(await screen.findByRole('switch', { name: '停用字典值 在籍' }))
		expect(updateDictionaryItem).not.toHaveBeenCalled()
		const dialog = await screen.findByRole('dialog', { name: /停用“在籍”字典值/ })
		expect(within(dialog).getByText(/现有记录仍保留原值/)).toBeInTheDocument()
		fireEvent.click(within(dialog).getByRole('button', { name: '确认停用' }))
		await waitFor(() => expect(vi.mocked(updateDictionaryItem).mock.calls[0]?.[0]).toEqual(expect.objectContaining({ id: 'item-1', enabled: false })))
	}, 10_000)
})
