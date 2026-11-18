import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../../shared/api/httpClient'
import {
  createDisciplineRuleDraft,
  getCurrentDisciplineRule,
  getDisciplineRuleVersions,
  publishDisciplineRuleDraft,
  saveDisciplineRuleDraft,
  validateDisciplineRuleDraft,
} from '../../features/discipline-config/api'
import { getCatalogs, getDictionaries } from '../../shared/api/adminApi'
import { DisciplineConfigPage } from './DisciplineConfigPage'

vi.mock('../../features/discipline-config/api', () => ({
  createDisciplineRuleDraft: vi.fn(),
  getCurrentDisciplineRule: vi.fn(),
  getDisciplineRuleVersions: vi.fn(),
  publishDisciplineRuleDraft: vi.fn(),
  saveDisciplineRuleDraft: vi.fn(),
  validateDisciplineRuleDraft: vi.fn(),
}))
vi.mock('../../shared/api/adminApi', () => ({ getCatalogs: vi.fn(), getDictionaries: vi.fn() }))

const makeVersion = (id: string, version: number, status: 'DRAFT' | 'PUBLISHED' | 'SUPERSEDED' = 'DRAFT') => ({
  id,
  version,
  status,
  rowVersion: 1,
  createdBy: 'admin',
  createdByName: '管理员',
  createdAt: '2026-11-16T08:00:00Z',
  publishedBy: status === 'PUBLISHED' ? 'admin' : undefined,
  publishedByName: status === 'PUBLISHED' ? '管理员' : undefined,
  publishedAt: status === 'PUBLISHED' ? '2026-11-16T08:01:00Z' : undefined,
  groups: [{
    key: 'attendance',
    name: '到课累计',
    unit: '次',
    terms: [{ inspectionTypeId: 'type-a', inspectionTypeName: '到课', sourceUnit: '次', operator: '+' as const, factor: 1 }],
    tiers: [
      { code: 'NOTICE', name: '通报批评', lower: 1, upper: 3, clause: '条款一', teacherConfirmationRequired: false },
      { code: 'WARNING', name: '警告', lower: 3, upper: 5, clause: '条款二', teacherConfirmationRequired: true },
      { code: 'SERIOUS_WARNING', name: '严重警告', lower: 5, upper: 7, clause: '条款三', teacherConfirmationRequired: true },
      { code: 'DEMERIT', name: '记过', lower: 7, upper: 9, clause: '条款四', teacherConfirmationRequired: true },
      { code: 'PROBATION', name: '留校察看', lower: 9, clause: '条款五', teacherConfirmationRequired: true },
    ],
  }],
})

function renderPage(path = '/discipline/config/edit', mode: 'view' | 'edit' = 'edit') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const routeElement = mode === 'edit'
    ? <Routes><Route path="/discipline/config/edit" element={<DisciplineConfigPage mode="edit" />} /><Route path="/discipline/config" element={<DisciplineConfigPage mode="view" />} /></Routes>
    : <DisciplineConfigPage mode="view" />
  return render(<MemoryRouter initialEntries={[path]}><QueryClientProvider client={client}><LocationProbe />{routeElement}</QueryClientProvider></MemoryRouter>)
}

function LocationProbe() {
  return <output data-testid="current-path">{useLocation().pathname}</output>
}

async function openStep(title: string) {
  if (title === '检查并发布') {
    fireEvent.click(await screen.findByText('配置处分档位', { selector: '.ant-steps-item-title' }))
    expect(screen.getAllByRole('button', { name: '通报批评' }).length).toBeGreaterThan(0)
  }
  fireEvent.click(await screen.findByText(title, { selector: '.ant-steps-item-title' }))
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(getCurrentDisciplineRule).mockResolvedValue({ current: null })
  vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [] })
  vi.mocked(getCatalogs).mockResolvedValue({ departments: [], organizationRoles: [], inspectionTypes: [{ id: 'type-a', code: 'ATTENDANCE_RATE', name: '到课', extensionKind: '', enabled: true, participatesInReport: true, sortOrder: 1 }] })
  vi.mocked(getDictionaries).mockResolvedValue({ types: [{ id: 'levels', code: 'DISCIPLINE_LEVEL', name: '处分档位', description: '', system: true, enabled: true }], items: [
    { id: 'notice', typeId: 'levels', value: 'NOTICE', label: '通报批评', description: '', sortOrder: 10, enabled: true },
    { id: 'warning', typeId: 'levels', value: 'WARNING', label: '警告', description: '', sortOrder: 20, enabled: true },
    { id: 'serious', typeId: 'levels', value: 'SERIOUS_WARNING', label: '严重警告', description: '', sortOrder: 30, enabled: true },
    { id: 'demerit', typeId: 'levels', value: 'DEMERIT', label: '记过', description: '', sortOrder: 40, enabled: true },
    { id: 'probation', typeId: 'levels', value: 'PROBATION', label: '留校察看', description: '', sortOrder: 50, enabled: true },
  ] })
  vi.mocked(validateDisciplineRuleDraft).mockResolvedValue({ valid: true })
})

describe('DisciplineConfigPage', () => {
  it('默认先展示当前生效配置，页面为只读并提供修改入口', async () => {
    const published = makeVersion('published-1', 1, 'PUBLISHED')
    vi.mocked(getCurrentDisciplineRule).mockResolvedValue({ current: published })
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [published] })
    renderPage('/discipline/config', 'view')

    expect(await screen.findByText(/当前生效配置 · 2026年11月16日/)).toBeInTheDocument()
    expect(screen.getByText('v1 · 当前生效')).toBeInTheDocument()
    expect(screen.getByRole('region', { name: '到课累计处分档位表格' })).toContainElement(screen.getByRole('columnheader', { name: '确认方式' }))
    expect(screen.getByRole('row', { name: /通报批评.*条款一.*1.*3/ })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: '到课累计所含检查项' }).textContent).toContain('到课')
    expect(screen.getByText('换算方式')).toBeInTheDocument()
    expect(screen.getByRole('region', { name: '到课累计处分档位' }).textContent).toContain('[1, 3) 次')
    expect(screen.getByRole('button', { name: '修改配置' })).toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: '通报批评处分名称' })).not.toBeInTheDocument()
    expect(screen.queryByRole('list', { name: '步骤' })).not.toBeInTheDocument()
  })

  it('查看页沿用最初发布的累计项顺序', async () => {
    const initial = { ...makeVersion('published-1', 1, 'SUPERSEDED'), groups: [
      { ...makeVersion('a', 1).groups[0], key: 'first', name: '先配置的累计项' },
      { ...makeVersion('b', 1).groups[0], key: 'second', name: '后配置的累计项' },
    ] }
    const current = { ...makeVersion('published-2', 2, 'PUBLISHED'), groups: [initial.groups[1], initial.groups[0]] }
    vi.mocked(getCurrentDisciplineRule).mockResolvedValue({ current })
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [current, initial] })
    renderPage('/discipline/config', 'view')

    await screen.findByText('先配置的累计项')
    expect([...document.querySelectorAll('.discipline-view-group-title > .ant-typography')].map((item) => item.textContent)).toEqual(['先配置的累计项', '后配置的累计项'])
  })

  it('在查看页打开版本历史后可以返回查看当前规则', async () => {
    const published = makeVersion('published-1', 1, 'PUBLISHED')
    vi.mocked(getCurrentDisciplineRule).mockResolvedValue({ current: published })
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [published] })
    renderPage('/discipline/config', 'view')

    fireEvent.click(await screen.findByRole('button', { name: '版本历史' }))
    expect(await screen.findByRole('region', { name: '处分规则版本历史' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '返回规则配置' }))
    expect(await screen.findByText(/当前生效配置 · 2026年11月16日/)).toBeInTheDocument()
  })

  it('发布成功后返回查看页且不再显示编辑底部操作', async () => {
    const draft = makeVersion('draft-1', 1)
    const published = makeVersion('published-1', 1, 'PUBLISHED')
    let currentCalls = 0
    let versionCalls = 0
    vi.mocked(getCurrentDisciplineRule).mockImplementation(async () => ({ current: currentCalls++ === 0 ? null : published }))
    vi.mocked(getDisciplineRuleVersions).mockImplementation(async () => ({ items: versionCalls++ === 0 ? [draft] : [published] }))
    vi.mocked(publishDisciplineRuleDraft).mockResolvedValue(published)
    renderPage()

    await openStep('检查并发布')
    fireEvent.click(screen.getByRole('button', { name: '校验并发布' }))
    fireEvent.click(await screen.findByRole('button', { name: '确认发布' }))

    await waitFor(() => expect(screen.getByTestId('current-path')).toHaveTextContent('/discipline/config'))
    expect(await screen.findByText(/当前生效配置 · 2026年11月16日/)).toBeInTheDocument()
    expect(document.querySelector('.discipline-step-footer')).not.toBeInTheDocument()
    expect(publishDisciplineRuleDraft).toHaveBeenCalledWith('draft-1', 1)
  })

  it('档位名称与依据默认只读，点击后才进入可保存或取消的编辑态', async () => {
    const draft = makeVersion('draft-1', 1)
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [draft] })
    renderPage()
    await openStep('配置处分档位')
    const tierRow = screen.getByRole('row', { name: /通报批评.*1.*3/ })

    expect(within(tierRow).queryByRole('textbox', { name: '通报批评处分名称' })).not.toBeInTheDocument()
    fireEvent.click(within(tierRow).getByRole('button', { name: '通报批评' }))
    const nameInput = within(tierRow).getByRole('textbox', { name: '通报批评处分名称' })
    expect(nameInput).toHaveValue('通报批评')
    expect(within(tierRow).getByRole('combobox', { name: '从处分档位字典填入名称' })).toBeInTheDocument()
    fireEvent.change(nameInput, { target: { value: '通报批评处分' } })
    fireEvent.click(screen.getAllByRole('button', { name: /保\s*存/ })[0])
    expect(await screen.findByRole('button', { name: '通报批评处分' })).toBeInTheDocument()

    fireEvent.click(within(tierRow).getByRole('button', { name: '条款一' }))
    const clauseEditor = within(tierRow).getByRole('textbox', { name: '通报批评处分依据条款' })
    expect(clauseEditor.tagName).toBe('TEXTAREA')
    fireEvent.change(clauseEditor, { target: { value: '未保存的改动' } })
    fireEvent.click(screen.getAllByRole('button', { name: /取\s*消/ })[0])
    expect(within(tierRow).getByRole('button', { name: '条款一' })).toBeInTheDocument()
  })

  it('处分档位工具栏只保留添加操作，确认开关使用标准尺寸', async () => {
    const draft = makeVersion('draft-1', 1)
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [draft] })
    renderPage()
    await openStep('配置处分档位')

    expect(screen.queryByRole('link', { name: '维护处分档位字典' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '添加档位' })).toBeInTheDocument()
    const confirmationSwitch = screen.getByRole('switch', { name: '通报批评需要老师确认' })
    expect(confirmationSwitch).not.toHaveClass('ant-switch-small')
    expect(confirmationSwitch).toHaveAttribute('aria-checked', 'false')
  })

  it('首版累计项可按需创建、归并检查项、移出成员并删除空组', async () => {
    const emptyDraft = { ...makeVersion('draft-1', 1), groups: [] }
    vi.mocked(createDisciplineRuleDraft).mockResolvedValue(emptyDraft)
    vi.mocked(getCatalogs).mockResolvedValue({
      departments: [],
      organizationRoles: [],
      inspectionTypes: [
        { id: 'type-a', code: 'ATTENDANCE_RATE', name: '到课', extensionKind: '', enabled: true, participatesInReport: true, sortOrder: 1 },
        { id: 'type-b', code: 'B', name: '晚归', extensionKind: '', enabled: true, participatesInReport: true, sortOrder: 2 },
      ],
    })
    renderPage()

    fireEvent.click(await screen.findByRole('button', { name: '新建规则草稿' }))
    expect(await screen.findByText('累计项（0）')).toBeInTheDocument()
    const keyboardMove = screen.getByRole('combobox', { name: '选择检查项' })
    expect(keyboardMove.closest('.discipline-grouping-controls')).toBeInTheDocument()
    expect(keyboardMove.closest('.discipline-group-list')).toBeNull()
    fireEvent.mouseDown(keyboardMove)
    expect(await screen.findByText('到课', { selector: '.ant-select-item-option-content' })).toBeInTheDocument()
    expect(screen.queryByText('到课（未归组）')).not.toBeInTheDocument()
    const groupTable = screen.getByRole('table')
    expect(keyboardMove.compareDocumentPosition(groupTable) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    const newGroupTarget = screen.getByText(/拖到这里，新建累计项/)
    fireEvent.drop(newGroupTarget, { dataTransfer: { getData: () => 'type-a' } })

    const groupName = await screen.findByRole('textbox', { name: '到课累计项名称' })
    expect(groupName).toHaveValue('到课')
    const groupRow = groupName.closest('tr') as HTMLElement
    fireEvent.drop(groupRow, { dataTransfer: { getData: () => 'type-b' } })
    expect(await within(groupRow).findByText('晚归')).toBeInTheDocument()
    const assignedSource = screen.getByText('晚归', { selector: '.discipline-source-chip strong' }).closest('.discipline-source-chip') as HTMLElement
    expect(assignedSource).toHaveAttribute('draggable', 'false')
    expect(within(assignedSource).getByText('已归入「到课」')).toBeInTheDocument()
    expect(screen.getByText(/全部启用检查维度（2）/)).toBeInTheDocument()

    const mergedGroupRow = screen.getByRole('textbox', { name: '到课累计项名称' }).closest('tr') as HTMLElement
    fireEvent.click(within(mergedGroupRow).getByRole('button', { name: '移出 晚归' }))
    expect(await screen.findByText('已归属 1 / 2 个启用维度，每个维度只能归属一个累计项。')).toBeInTheDocument()
    const remainingGroupRow = screen.getByRole('textbox', { name: '到课累计项名称' }).closest('tr') as HTMLElement
    fireEvent.click(within(remainingGroupRow).getByRole('button', { name: '删除累计项' }))
    expect(await screen.findByText('累计项（0）')).toBeInTheDocument()
    expect(screen.getByText('到课', { selector: '.discipline-source-chip strong' })).toBeInTheDocument()
    expect(screen.getByText('晚归', { selector: '.discipline-source-chip strong' })).toBeInTheDocument()
  })

  it('保存失败时保留草稿输入并阻止规则校验和发布', async () => {
    const draft = makeVersion('draft-1', 1)
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [draft] })
    vi.mocked(saveDisciplineRuleDraft).mockRejectedValue(new ApiError(409, 'CONFLICT', '版本冲突'))
    renderPage()

    fireEvent.change(await screen.findByRole('textbox', { name: '到课累计累计项名称' }), { target: { value: '到课规则' } })
    await openStep('检查并发布')
    fireEvent.click(screen.getByRole('button', { name: '校验并发布' }))

    await waitFor(() => expect(saveDisciplineRuleDraft).toHaveBeenCalledTimes(1))
    expect(validateDisciplineRuleDraft).not.toHaveBeenCalled()
    expect(publishDisciplineRuleDraft).not.toHaveBeenCalled()
    await openStep('归组并命名')
    expect(screen.getByRole('textbox', { name: '到课规则累计项名称' })).toHaveValue('到课规则')
    expect(await screen.findByText(/保存失败，输入内容已保留/)).toBeInTheDocument()
  })

  it('从历史只读页返回后新建草稿并立即选中新草稿', async () => {
    const previous = makeVersion('published-1', 1, 'PUBLISHED')
    const created = makeVersion('draft-2', 2)
    vi.mocked(getCurrentDisciplineRule).mockResolvedValue({ current: previous })
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [previous] })
    vi.mocked(createDisciplineRuleDraft).mockResolvedValue(created)
    renderPage()

    fireEvent.click(await screen.findByRole('button', { name: '版本历史' }))
    fireEvent.click(screen.getByRole('button', { name: 'v1' }))
    expect(await screen.findByText('规则版本 v1 · 只读')).toBeInTheDocument()
    expect(screen.queryByPlaceholderText('输入累计项名称')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '返回规则配置' }))
    fireEvent.click(await screen.findByRole('button', { name: '新建规则草稿' }))
    expect(await screen.findByRole('heading', { name: '把参与同一项累计的检查项放到一起' })).toBeInTheDocument()
    expect(await screen.findByDisplayValue('到课累计')).toBeInTheDocument()
  })

  it('按处分字典类型 id 加载字典并可移除末尾档位', async () => {
    const draft = makeVersion('draft-1', 1)
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [draft] })
    renderPage()
    await openStep('配置处分档位')
    await waitFor(() => expect(getDictionaries).toHaveBeenCalledWith('levels'))
    expect(screen.getByText('适用检查项：到课').closest('.ant-card-head')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '留校察看' })).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: '通报批评需要老师确认' })).toHaveAttribute('aria-checked', 'false')
    expect(document.getElementById('discipline-groups-0--tiers-0--lower')).toHaveAttribute('step', '1')
    expect(document.getElementById('discipline-groups-0--tiers-0--lower')).toHaveValue('1')
    expect(screen.getByRole('region', { name: '处分档位表，可横向滚动' })).toBeInTheDocument()
    const removeButtons = screen.getAllByRole('button', { name: '移除' })
    expect(removeButtons).toHaveLength(5)
    fireEvent.click(removeButtons[4])
    expect(screen.queryByRole('button', { name: '留校察看' })).not.toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: '移除' })).toHaveLength(4)
  })

  it('换算系数使用整数输入', async () => {
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [makeVersion('draft-1', 1)] })
    renderPage()
    await openStep('确认计量与换算')
    const factor = document.getElementById('discipline-groups-0--terms-0--factor')
    expect(factor).toHaveAttribute('step', '1')
    expect(factor).toHaveValue('1')
  })

  it('删除中间档位后保持相邻区间连续', async () => {
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [makeVersion('draft-1', 1)] })
    renderPage()
    await openStep('配置处分档位')
    const updatedRemoveButtons = screen.getAllByRole('button', { name: '移除' })
    fireEvent.click(updatedRemoveButtons[2])
    expect(document.getElementById('discipline-groups-0--tiers-1--upper')).toHaveValue('7')
    expect(document.getElementById('discipline-groups-0--tiers-2--lower')).toHaveValue('7')
  })

  it('新增末尾档位先要求用户填写阈值并以同一值闭合区间', async () => {
    const draft = makeVersion('draft-1', 1)
    draft.groups[0].tiers = [
      { code: 'NOTICE', name: '通报批评', lower: 1, upper: 3, clause: '条款一', teacherConfirmationRequired: false },
      { code: 'WARNING', name: '警告', lower: 3, clause: '条款二', teacherConfirmationRequired: true },
    ]
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [draft] })
    vi.mocked(getDictionaries).mockResolvedValue({
      types: [{ id: 'levels', code: 'DISCIPLINE_LEVEL', name: '处分档位', description: '', system: true, enabled: true }],
      items: [
        { id: 'notice', typeId: 'levels', value: 'NOTICE', label: '通报批评', description: '', sortOrder: 10, enabled: true },
        { id: 'warning', typeId: 'levels', value: 'WARNING', label: '警告', description: '', sortOrder: 20, enabled: true },
        { id: 'custom', typeId: 'levels', value: 'CUSTOM', label: '附加档', description: '', sortOrder: 30, enabled: true },
      ],
    })
    renderPage()
    await openStep('配置处分档位')
    fireEvent.click(await screen.findByRole('button', { name: '添加档位' }))
    const dialog = await screen.findByRole('dialog', { name: '设置新增档位起始值' })
    fireEvent.change(within(dialog).getByRole('spinbutton', { name: '新档起始值' }), { target: { value: '9' } })
    fireEvent.click(within(dialog).getByRole('button', { name: '确认并添加' }))
    await waitFor(() => expect(document.getElementById('discipline-groups-0--tiers-1--upper')).toHaveValue('9'))
    expect(document.getElementById('discipline-groups-0--tiers-2--lower')).toHaveValue('9')
    expect(document.getElementById('discipline-groups-0--tiers-2--upper')).toHaveValue('')
  })

  it('新增档位遵循启用字典项的动态顺序并允许自定义名称及快捷填入', async () => {
    const draft = makeVersion('draft-1', 1)
    draft.groups[0].tiers = [{ code: 'NOTICE', name: '通报批评', lower: 1, clause: '原档条款', teacherConfirmationRequired: false }]
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [draft] })
    vi.mocked(getDictionaries).mockResolvedValue({
      types: [{ id: 'levels', code: 'DISCIPLINE_LEVEL', name: '处分档位', description: '', system: true, enabled: true }],
      items: [
        { id: 'custom', typeId: 'levels', value: 'CUSTOM_LEVEL', label: '附加观察档', description: '', sortOrder: 5, enabled: true },
        { id: 'notice', typeId: 'levels', value: 'NOTICE', label: '通报批评', description: '', sortOrder: 10, enabled: true },
        { id: 'warning', typeId: 'levels', value: 'WARNING', label: '警告', description: '', sortOrder: 20, enabled: true },
      ],
    })
    renderPage()
    await openStep('配置处分档位')
    await waitFor(() => expect(getDictionaries).toHaveBeenCalledWith('levels'))
    fireEvent.click(await screen.findByRole('button', { name: '添加档位' }))
    const addedTierRow = screen.getByRole('row', { name: /附加观察档/ })
    expect(within(addedTierRow).getByRole('button', { name: '附加观察档' })).toBeInTheDocument()
    expect(screen.queryByText('CUSTOM_LEVEL')).not.toBeInTheDocument()
    expect(document.getElementById('discipline-groups-0--tiers-0--lower')).toHaveValue('0')
    expect(document.getElementById('discipline-groups-0--tiers-0--upper')).toHaveValue('1')
    fireEvent.click(within(addedTierRow).getByRole('button', { name: '附加观察档' }))
    const nameInput = screen.getByRole('textbox', { name: '附加观察档处分名称' })
    fireEvent.change(nameInput, { target: { value: '学院特别处分' } })
    expect(nameInput).toHaveValue('学院特别处分')
    fireEvent.click(screen.getAllByRole('button', { name: /保\s*存/ })[0])
    expect(await screen.findByRole('button', { name: '学院特别处分' })).toBeInTheDocument()
  })

  it('处分档位系统字典未初始化时明确提示迁移，不伪装为空白下拉', async () => {
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [makeVersion('draft-1', 1)] })
    vi.mocked(getDictionaries).mockResolvedValue({ types: [], items: [] })
    renderPage()
    await openStep('配置处分档位')
    expect(await screen.findByText('处分档位字典未初始化')).toBeInTheDocument()
    expect(screen.getByText(/确认服务端已加载迁移 065/)).toBeInTheDocument()
  })

  it('存量草稿来源单位错误时仍按检查目录回显权威值', async () => {
    const draft = makeVersion('draft-1', 1)
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [draft] })
    renderPage()
    await openStep('确认计量与换算')
    expect(await screen.findByDisplayValue('学时')).toBeInTheDocument()
  })

  it('发布前摘要分开展示检查项、换算和档位依据', async () => {
    const draft = makeVersion('draft-1', 1)
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [draft] })
    renderPage()
    await openStep('检查并发布')
    expect(screen.getByRole('region', { name: '到课累计检查项' })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: '到课累计换算公式' })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: '到课累计处分档位' })).toBeInTheDocument()
    expect(screen.getByText('条款一')).toBeInTheDocument()
    expect(document.querySelectorAll('.discipline-summary-tier')).toHaveLength(5)
    expect(document.querySelector('.discipline-rule-summary')?.querySelector('.discipline-summary-tiers')).toBeInTheDocument()
    expect(screen.queryByText('发布影响范围')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '修改归组' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '修改换算' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '修改档位' })).not.toBeInTheDocument()
    expect(document.querySelector('.discipline-rule-summary')).toHaveClass('discipline-rule-summary')
  })

  it('离开处分档位步骤前拦截错误区间，修正后才可继续', async () => {
    const draft = makeVersion('draft-1', 1)
    draft.groups[0].tiers[1].lower = 4
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [draft] })
    renderPage()

    await openStep('配置处分档位')
    expect(document.body.textContent).toContain('通报批评')
    const continueButton = () => Array.from(document.querySelectorAll('.discipline-step-footer button')).find((button) => button.textContent?.includes('继续：检查并发布')) as HTMLButtonElement
    fireEvent.click(continueButton())

    expect(document.body.textContent).toContain('第 3 步“配置处分档位”未通过')
    const tierStep = Array.from(document.querySelectorAll('.ant-steps-item')).find((item) => item.textContent?.includes('配置处分档位'))
    const publishStep = Array.from(document.querySelectorAll('.ant-steps-item')).find((item) => item.textContent?.includes('检查并发布'))
    expect(tierStep).toHaveClass('ant-steps-item-error', 'ant-steps-item-active')
    expect(publishStep).not.toHaveClass('ant-steps-item-process')
    expect(document.getElementById('discipline-groups-0--tiers-1--lower')?.closest('.ant-form-item')?.textContent).toContain('处分区间存在空档或重叠')
    expect(validateDisciplineRuleDraft).not.toHaveBeenCalled()

    fireEvent.change(document.getElementById('discipline-groups-0--tiers-1--lower')!, { target: { value: '3' } })
    expect(document.getElementById('discipline-groups-0--tiers-1--lower')).toHaveValue('3')
    fireEvent.click(continueButton())

    expect(Array.from(document.querySelectorAll('.ant-steps-item')).find((item) => item.textContent?.includes('检查并发布'))).toHaveClass('ant-steps-item-process')
    expect(Array.from(document.querySelectorAll('button.ant-btn-primary')).some((button) => button.textContent?.includes('校验并发布'))).toBe(true)
    expect(validateDisciplineRuleDraft).not.toHaveBeenCalled()
  })

  it('处分档位为空时在当前步骤显示具体错误', async () => {
    const draft = makeVersion('draft-1', 1)
    draft.groups[0].tiers = []
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [draft] })
    renderPage()

    await openStep('配置处分档位')
    const continueButton = () => Array.from(document.querySelectorAll('.discipline-step-footer button')).find((button) => button.textContent?.includes('继续：检查并发布')) as HTMLButtonElement
    fireEvent.click(continueButton())

    expect(document.body.textContent).toContain('第 3 步“配置处分档位”未通过')
    expect(screen.getByText('至少保留一个处分档位')).toHaveAttribute('role', 'alert')
    expect(screen.getByText('配置处分档位').closest('.ant-steps-item')).toHaveClass('ant-steps-item-error', 'ant-steps-item-active')
  })

  it('服务端返回无索引档位错误时将 Step 3 标记为错误且不自动离开发布页', async () => {
    const draft = makeVersion('draft-1', 1)
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [draft] })
    vi.mocked(validateDisciplineRuleDraft).mockRejectedValue(new ApiError(400, 'INVALID_DISCIPLINE_RULE', '缺少档位', [
      { path: 'groups[0].tiers', code: 'TIER_REQUIRED', message: '至少保留一个处分档位' },
    ]))
    renderPage()

    await openStep('配置处分档位')
    expect(screen.getAllByRole('button', { name: '通报批评' }).length).toBeGreaterThan(0)
    await openStep('检查并发布')
    fireEvent.click(screen.getByRole('button', { name: '校验并发布' }))

    expect(await screen.findByText(/规则校验未通过：第 3 步“配置处分档位”发现 1 项需要修改/)).toBeInTheDocument()
    expect(screen.getByText('检查并发布').closest('.ant-steps-item')).toHaveClass('ant-steps-item-process')
    expect(screen.getByText('配置处分档位').closest('.ant-steps-item')).toHaveClass('ant-steps-item-error')
  })

  it('校验失败只提示并标记问题步骤，手动返回后保留字段错误', async () => {
    const draft = makeVersion('draft-1', 1)
    vi.mocked(getDisciplineRuleVersions).mockResolvedValue({ items: [draft] })
    vi.mocked(validateDisciplineRuleDraft).mockRejectedValue(new ApiError(400, 'INVALID_DISCIPLINE_RULE', '字段错误', [
      { path: 'groups[0].tiers[0].clause', code: 'CLAUSE_REQUIRED', message: '请填写处分依据条款' },
      { path: 'groups[0].tiers[0].lower', code: 'TIER_GAP', message: '起始值与上一档位不连续' },
    ]))
    renderPage()

    await openStep('配置处分档位')
    await openStep('检查并发布')
    fireEvent.click(screen.getByRole('button', { name: '校验并发布' }))
    expect(await screen.findByText(/规则校验未通过：第 3 步“配置处分档位”发现 2 项需要修改/)).toBeInTheDocument()
    expect(screen.getByText('检查并发布').closest('.ant-steps-item')).toHaveClass('ant-steps-item-process')
    expect(screen.getByText('配置处分档位').closest('.ant-steps-item')).toHaveClass('ant-steps-item-error')
    expect(document.querySelector('.discipline-validation-summary')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '前往修正' })).not.toBeInTheDocument()
    expect(screen.queryByText('请填写处分依据条款')).not.toBeInTheDocument()

    await openStep('配置处分档位')
    const clauseButton = await screen.findByRole('button', { name: '条款一' })
    const clauseItem = clauseButton.closest('.ant-form-item') as HTMLElement
    const lowerIssue = screen.getAllByText('起始值与上一档位不连续').find((element) => element.closest('.ant-form-item'))
    const lowerItem = lowerIssue?.closest('.ant-form-item') as HTMLElement
    expect(within(clauseItem).getByText('请填写处分依据条款')).toBeInTheDocument()
    expect(lowerItem).toHaveClass('ant-form-item-has-error')

    fireEvent.click(clauseButton)
    const clauseInput = screen.getByRole('textbox', { name: '通报批评依据条款' })
    fireEvent.change(clauseInput, { target: { value: '纪律条例第一条' } })
    fireEvent.click(screen.getAllByRole('button', { name: /保\s*存/ })[0])
    await waitFor(() => expect(clauseItem).not.toHaveClass('ant-form-item-has-error'))
    expect(lowerItem).toHaveClass('ant-form-item-has-error')
    expect(within(lowerItem).getByText('起始值与上一档位不连续')).toBeInTheDocument()
  })
})
