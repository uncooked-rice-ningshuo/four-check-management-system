import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { Modal } from 'antd'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RegisterPage } from '../auth/RegisterPage'
import { RegisterResultPage } from '../auth/RegisterResultPage'
import { DictionaryPage } from './DictionaryPage'
import { MasterDataPage } from './MasterDataPage'
import { OrganizationConfigPage } from './OrganizationConfigPage'
import { RegistrationReviewPage } from './RegistrationReviewPage'
import { UserManagementPage } from './UserManagementPage'
import { assignUserRole, getCatalogs, getDepartmentInspectionScopes, getDictionaries, getMasterData, getPendingRegistrations, getUsers, reviewRegistration, updateDepartment, updateDepartmentInspectionScope, updateDictionaryItem, updateInspectionType } from '../../shared/api/adminApi'

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
  getDictionaries: vi.fn().mockResolvedValue({ types: [], items: [] }),
  createDictionaryType: vi.fn(), updateDictionaryType: vi.fn(), createDictionaryItem: vi.fn(), updateDictionaryItem: vi.fn(),
}))

vi.mock('../../shared/api/authApi', () => ({ registerAccount: vi.fn() }))

const renderPage = (element: React.ReactNode) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><MemoryRouter>{element}</MemoryRouter></QueryClientProvider>)
}

describe('阶段一管理页面', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getUsers).mockResolvedValue([])
    vi.mocked(getPendingRegistrations).mockResolvedValue([])
    vi.mocked(getCatalogs).mockResolvedValue({ departments: [], organizationRoles: [], inspectionTypes: [] })
	vi.mocked(getDepartmentInspectionScopes).mockResolvedValue([])
    vi.mocked(getMasterData).mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
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
  })

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

  it('归属保存成功后清除草稿并禁用保存按钮', async () => {
    vi.mocked(getUsers).mockResolvedValue([{ accountId: 'user-1', loginName: '24010001', studentNo: '24010001', name: '张三', status: 'ACTIVE', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false, departmentId: 'department-1', organizationRoleId: 'role-1' }])
    vi.mocked(getCatalogs).mockResolvedValue({ departments: [{ id: 'department-1', code: 'ONE', name: '部门一', enabled: true }, { id: 'department-2', code: 'TWO', name: '部门二', enabled: true }], organizationRoles: [{ id: 'role-1', code: 'MEMBER', name: '委员', enabled: true, sortOrder: 1 }], inspectionTypes: [] })
    vi.mocked(assignUserRole).mockResolvedValue(undefined)
    renderPage(<UserManagementPage />)

    const saveButton = await screen.findByRole('button', { name: '保存归属' })
    expect(saveButton).toBeDisabled()
    fireEvent.mouseDown(screen.getByLabelText('张三所属部门'))
    fireEvent.click(await screen.findByText('部门二'))
    expect(saveButton).toBeEnabled()
    fireEvent.click(saveButton)

    await waitFor(() => expect(assignUserRole).toHaveBeenCalled())
    await waitFor(() => expect(saveButton).toBeDisabled())
  })

  it('基础数据页面使用分类页签和 ProTable 新增入口', async () => {
    renderPage(<MasterDataPage />)
    expect(await screen.findByRole('tab', { name: '学生' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '班级' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新增学生' })).toBeInTheDocument()
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
    fireEvent.mouseDown(screen.getByLabelText('性别'))
    expect(await screen.findByText('男（系统字典）')).toBeInTheDocument()
    fireEvent.mouseDown(screen.getByLabelText('学籍状态'))
    expect((await screen.findAllByText('在籍（系统字典）')).length).toBeGreaterThan(0)
  })

  it('字典页面提供类型与选项的配置入口', async () => {
    renderPage(<DictionaryPage />)
    expect(await screen.findByRole('button', { name: '新增字典类型' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '新增字典类型' }))
    expect(await screen.findByRole('dialog', { name: '新增字典类型' })).toBeInTheDocument()
    expect(screen.getByLabelText('类型编码')).toBeInTheDocument()
  })

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
	})
})
