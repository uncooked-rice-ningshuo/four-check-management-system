import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, App, Button, Drawer, Empty, Modal, Select, Spin, Switch, Typography } from 'antd'
import type { DepartmentDefaultPermissions, DepartmentInspectionScope, DepartmentReportScope, InspectionRoleDefaults, OrganizationCatalogs, ReportRoleDefaults } from '../../entities/organization/model/organization'
import { getDepartmentDefaults, getUsers, saveDepartmentDefaults } from '../../shared/api/adminApi'
import { ApiError } from '../../shared/api/httpClient'
import './department-defaults.css'

const inspectionRoleCodes = ['MEMBER', 'LEADERSHIP', 'PRESIDIUM'] as const
const reportRoleCodes = ['MEMBER', 'MINISTER', 'VICE_MINISTER'] as const
const inspectionActions = [
  ['canView', '查看记录'], ['canEnter', '录入与提交'], ['canManage', '管理记录'], ['canReview', '审核批次'],
] as const
const reportActions = [
  ['canView', '查看周期与汇总'], ['canGenerate', '生成草稿'], ['canCorrect', '草稿修正'],
  ['canReview', '审核'], ['canReturn', '退回'], ['canFinalize', '定稿'],
  ['canOutputSettings', '文件输出设置'], ['canArchive', '查看归档'],
] as const
const roleName = (code: string) => ({ MEMBER: '委员', LEADERSHIP: '部长 / 副部长', PRESIDIUM: '主席团', MINISTER: '部长', VICE_MINISTER: '副部长' })[code as 'MEMBER' | 'LEADERSHIP' | 'PRESIDIUM' | 'MINISTER' | 'VICE_MINISTER'] ?? code
const inspectionShortNames = { canView: '查看', canEnter: '录入', canManage: '管理', canReview: '审核' } as const
const reportShortNames = { canView: '查看', canGenerate: '草稿', canCorrect: '修正', canReview: '审核', canReturn: '退回', canFinalize: '定稿', canOutputSettings: '输出设置', canArchive: '归档' } as const

function enabledActions(scope: Record<keyof typeof inspectionShortNames, boolean>, role: Record<keyof typeof inspectionShortNames, boolean>) {
  return inspectionActions.filter(([key]) => scope[key] && role[key]).map(([key]) => inspectionShortNames[key]).join('、') || '无'
}

function enabledReportActions(scope: DepartmentReportScope, role: ReportRoleDefaults) {
  return reportActions.filter(([key]) => scope[key] && role[key]).map(([key]) => reportShortNames[key]).join('、') || '无'
}

function normalize(source: DepartmentDefaultPermissions, catalogs: OrganizationCatalogs): DepartmentDefaultPermissions {
  return {
    ...source,
    scopes: catalogs.inspectionTypes.map(type => source.scopes.find(scope => scope.inspectionTypeId === type.id) ?? {
      departmentId: source.departmentId, inspectionTypeId: type.id, canView: false, canEnter: false, canManage: false, canReview: false, canExport: false, enabled: false,
    }),
    inspectionRoles: catalogs.inspectionTypes.flatMap(type => inspectionRoleCodes.map(roleCode => source.inspectionRoles.find(role => role.inspectionTypeId === type.id && role.roleCode === roleCode) ?? {
      inspectionTypeId: type.id, roleCode, canView: false, canEnter: false, canManage: false, canReview: false, canExport: false,
    })),
    reportRoles: reportRoleCodes.map(roleCode => source.reportRoles.find(role => role.roleCode === roleCode) ?? {
      roleCode, canView: false, canGenerate: false, canCorrect: false, canReview: false, canReturn: false,
      canFinalize: false, canPublish: false, canOutputSettings: false, canArchive: false,
    }),
  }
}

function changes(before: DepartmentDefaultPermissions, after: DepartmentDefaultPermissions, catalogs: OrganizationCatalogs): string[] {
  const result: string[] = []
  for (const [index, scope] of after.scopes.entries()) {
    const previous = before.scopes[index]
    const type = catalogs.inspectionTypes.find(item => item.id === scope.inspectionTypeId)?.name ?? scope.inspectionTypeId
    for (const [key, label] of [['enabled', '承担此检查'], ...inspectionActions] as const) {
      if (previous[key] !== scope[key]) result.push(`${type} · ${label}：${scope[key] ? '开启' : '关闭'}`)
    }
  }
  for (const [index, role] of after.inspectionRoles.entries()) {
    const previous = before.inspectionRoles[index]
    for (const [key, label] of inspectionActions) {
      if (previous[key] !== role[key]) {
        const scope = after.scopes.find(item => item.inspectionTypeId === role.inspectionTypeId)
        result.push(`${catalogs.inspectionTypes.find(item => item.id === role.inspectionTypeId)?.name ?? role.inspectionTypeId} · ${roleName(role.roleCode)} · ${label}：${role[key] ? scope?.enabled && scope[key] ? '默认生效' : '已配置，暂不生效' : '关闭'}`)
      }
    }
  }
  for (const [key, label] of [['enabled', '承担周通报'], ...reportActions] as const) {
    if (before.reportScope[key] !== after.reportScope[key]) result.push(`周通报 · 部门功能 · ${label}：${after.reportScope[key] ? '开启' : '关闭'}`)
  }
  for (const [index, role] of after.reportRoles.entries()) {
    const previous = before.reportRoles[index]
    for (const [key, label] of reportActions) {
      if (previous[key] !== role[key]) result.push(`周通报 · ${roleName(role.roleCode)} · ${label}：${role[key] ? after.reportScope.enabled && after.reportScope[key] ? '默认生效' : '已配置，暂不生效' : '关闭'}`)
    }
  }
  return result
}

export function DepartmentDefaultsPanel({ catalogs }: { catalogs: OrganizationCatalogs }) {
  const { message } = App.useApp()
  const [modal, modalContextHolder] = Modal.useModal()
  const client = useQueryClient()
  const [departmentId, setDepartmentId] = useState(() => catalogs.departments[0]?.id ?? '')
  const [drafts, setDrafts] = useState<Record<string, DepartmentDefaultPermissions>>({})
  const [editor, setEditor] = useState<{ kind: 'scope' | 'inspectionRoles'; inspectionTypeId: string } | { kind: 'reportScope' | 'reportRoles' } | null>(null)
  const department = catalogs.departments.find(item => item.id === departmentId)
  const selectedId = department?.id ?? catalogs.departments[0]?.id ?? ''
  const selectedDepartment = catalogs.departments.find(item => item.id === selectedId)
  const configQuery = useQuery({ queryKey: ['department-defaults', selectedId], queryFn: async () => {
    const result = await getDepartmentDefaults(selectedId)
    if (!result.reportScope) throw new Error('部门权限响应缺少周通报配置')
    return result
  }, enabled: Boolean(selectedId), retry: false })
  const usersQuery = useQuery({ queryKey: ['admin-users'], queryFn: getUsers, retry: false })
  const baseline = configQuery.data && selectedDepartment ? normalize(configQuery.data, catalogs) : null
  const draft = drafts[selectedId] ?? baseline
  const changeList = baseline && draft ? changes(baseline, draft, catalogs) : []
  const affectedUsers = usersQuery.data?.filter(user => user.departmentId === selectedId).length
  const save = useMutation({ mutationFn: saveDepartmentDefaults, onSuccess: result => {
    client.setQueryData(['department-defaults', result.departmentId], result)
    setDrafts(previous => { const next = { ...previous }; delete next[result.departmentId]; return next })
    void client.invalidateQueries({ queryKey: ['department-defaults', result.departmentId] })
    void client.invalidateQueries({ queryKey: ['effective-permissions'] })
    message.success('部门默认权限已保存')
  }, onError: error => {
    if (error instanceof ApiError && error.code === 'DEPARTMENT_PERMISSIONS_VERSION_CONFLICT') {
      message.error('配置已被其他人更新。请刷新后核对差异，你的未保存修改仍保留。')
    } else message.error(error.message)
  } })
  const update = (next: DepartmentDefaultPermissions) => setDrafts(previous => ({ ...previous, [selectedId]: next }))
  const updateScope = (scope: DepartmentInspectionScope, key: 'enabled' | 'canView' | 'canEnter' | 'canManage' | 'canReview', value: boolean) => {
    if (!draft) return
    update({ ...draft, scopes: draft.scopes.map(item => item.inspectionTypeId === scope.inspectionTypeId ? { ...item, [key]: value } : item) })
  }
  const updateInspectionRole = (role: InspectionRoleDefaults, key: keyof Omit<InspectionRoleDefaults, 'roleCode'>, value: boolean) => {
    if (!draft) return
    update({ ...draft, inspectionRoles: draft.inspectionRoles.map(item => item.inspectionTypeId === role.inspectionTypeId && item.roleCode === role.roleCode ? { ...item, [key]: value } : item) })
  }
  const updateReportRole = (role: ReportRoleDefaults, key: keyof Omit<ReportRoleDefaults, 'roleCode'>, value: boolean) => {
    if (!draft) return
    update({ ...draft, reportRoles: draft.reportRoles.map(item => item.roleCode === role.roleCode ? { ...item, [key]: value } : item) })
  }
  const updateReportScope = (key: keyof DepartmentReportScope, value: boolean) => {
    if (!draft) return
    update({ ...draft, reportScope: { ...draft.reportScope, [key]: value } })
  }
  const reportRoleName = roleName
  const editingScope = editor && (editor.kind === 'scope' || editor.kind === 'inspectionRoles') ? draft?.scopes.find(item => item.inspectionTypeId === editor.inspectionTypeId) : undefined
  const editingType = editor && (editor.kind === 'scope' || editor.kind === 'inspectionRoles') ? catalogs.inspectionTypes.find(item => item.id === editor.inspectionTypeId) : undefined
  const editingInspectionRoles = editor?.kind === 'inspectionRoles' ? draft?.inspectionRoles.filter(role => role.inspectionTypeId === editor.inspectionTypeId) ?? [] : []
  const reportEnabled = draft?.reportScope.enabled ?? false
  const actionSummary = (scope: DepartmentInspectionScope) => !scope.enabled ? '未承担' :
    inspectionActions.filter(([key]) => scope[key]).map(([key]) => inspectionShortNames[key]).join('、') || '未开放动作'
  const reportSummary = !reportEnabled ? '未承担' : reportActions.filter(([key]) => draft?.reportScope[key]).map(([key]) => reportShortNames[key]).join('、') || '未开放动作'
  const confirmSave = () => {
    if (!draft || !changeList.length) return
    modal.confirm({ title: `保存“${selectedDepartment?.name}”默认权限？`, width: 620, okText: '保存全部修改', cancelText: '继续编辑',
      content: <div><Typography.Paragraph>本次将修改 {changeList.length} 项；{affectedUsers === undefined ? '受影响人数未能读取' : `当前主部门为此部门的账号有 ${affectedUsers} 人`}。保存后这些账号的会话会失效。部门动作关闭时，同名岗位动作即使已配置也不再默认生效。</Typography.Paragraph>
        <ul className="department-defaults-change-list">{changeList.map(item => <li key={item}>{item}</li>)}</ul></div>,
      onOk: () => save.mutateAsync(draft),
    })
  }
  if (!catalogs.departments.length) return <Empty description="暂无部门，请先新增部门" />
  return <div className="department-defaults-layout">{modalContextHolder}
    <header className="department-defaults-heading" aria-label="部门权限配置操作">
      <div className="department-defaults-controls"><label htmlFor="department-defaults-select">当前部门</label>
        <Select id="department-defaults-select" aria-label="选择配置部门" value={selectedId} onChange={value => { setDepartmentId(value); setEditor(null) }}
          options={catalogs.departments.map(item => ({ value: item.id, label: `${item.name}${drafts[item.id] ? ' · 未保存' : ''}` }))} />
        <span className="department-defaults-version">配置版本 {draft?.version ?? '—'}</span>
        <Button type="primary" disabled={!changeList.length || save.isPending || !usersQuery.isSuccess} loading={save.isPending} onClick={confirmSave}>保存修改{changeList.length ? ` (${changeList.length})` : ''}</Button>
      </div>
    </header>
    {configQuery.isPending && <div role="status"><Spin /> 正在加载部门权限</div>}
    {configQuery.isError && <Alert type="error" showIcon title="部门权限加载失败" description="请重试后再编辑，当前没有将读取失败当成空配置。" action={<Button onClick={() => void configQuery.refetch()}>重试</Button>} />}
    {usersQuery.isError && <Alert type="error" showIcon title="无法核对受影响人数" action={<Button onClick={() => void usersQuery.refetch()}>重试</Button>} />}
    {save.isError && <Alert type="error" showIcon title="保存失败，修改仍保留" description={save.error.message} action={save.error instanceof ApiError && save.error.code === 'DEPARTMENT_PERMISSIONS_VERSION_CONFLICT' ? <Button onClick={() => void configQuery.refetch()}>读取最新配置</Button> : undefined} />}
    {draft && baseline && <>
      <div className="department-defaults-compact">
        <section className="department-defaults-section" aria-labelledby="department-business-title">
          <div className="department-defaults-section-heading"><div><Typography.Title id="department-business-title" level={3}>日常检查</Typography.Title>
            <Typography.Text type="secondary">按检查项设置部门动作和岗位默认权限</Typography.Text></div>
            <span className="department-defaults-count">承担 {draft.scopes.filter(item => item.enabled).length} / {draft.scopes.length} 项</span></div>
          <div className="department-defaults-table-scroll" role="region" aria-label="日常检查权限表" tabIndex={0}><table className="department-defaults-table department-defaults-list">
            <thead><tr><th scope="col">检查项</th><th scope="col">部门承担</th><th scope="col">部门功能</th><th scope="col">岗位信息</th><th scope="col">设置</th></tr></thead>
            <tbody>{draft.scopes.map(scope => {
              const type = catalogs.inspectionTypes.find(item => item.id === scope.inspectionTypeId)
              const roles = draft.inspectionRoles.filter(role => role.inspectionTypeId === scope.inspectionTypeId)
              return <tr key={scope.inspectionTypeId}><th scope="row">{type?.name ?? scope.inspectionTypeId}</th>
                <td className="department-defaults-assignment"><Switch size="small" checked={scope.enabled} disabled={save.isPending} onChange={value => updateScope(scope, 'enabled', value)} aria-label={`${selectedDepartment?.name} ${type?.name} 承担此检查`} /><span>{scope.enabled ? '承担' : '未承担'}</span></td>
                <td className="department-defaults-action-summary">{actionSummary(scope)}</td>
                <td className="department-defaults-role-summary">{roles.map(role => <div key={role.roleCode} className="department-defaults-role-line"><span>{roleName(role.roleCode)}</span><span>{scope.enabled ? enabledActions(scope, role) : '无'}</span></div>)}</td>
                <td><div className="department-defaults-row-actions"><Button type="link" onClick={() => setEditor({ kind: 'scope', inspectionTypeId: scope.inspectionTypeId })} aria-label={`配置${type?.name ?? scope.inspectionTypeId}部门功能`}>部门功能</Button>
                  <Button type="link" onClick={() => setEditor({ kind: 'inspectionRoles', inspectionTypeId: scope.inspectionTypeId })} aria-label={`配置${type?.name ?? scope.inspectionTypeId}角色权限`}>角色权限</Button></div></td></tr>
            })}</tbody></table></div>
        </section>
        <section className="department-defaults-section" aria-labelledby="department-report-title">
          <div className="department-defaults-section-heading"><div><Typography.Title id="department-report-title" level={3}>周通报</Typography.Title>
            <Typography.Text type="secondary">与日常检查使用相同的配置方式</Typography.Text></div></div>
          <div className="department-defaults-table-scroll" role="region" aria-label="周通报权限表" tabIndex={0}><table className="department-defaults-table department-defaults-list">
            <thead><tr><th scope="col">业务项</th><th scope="col">部门承担</th><th scope="col">部门功能</th><th scope="col">岗位信息</th><th scope="col">设置</th></tr></thead>
            <tbody><tr><th scope="row">周通报</th><td className="department-defaults-assignment"><Switch size="small" checked={reportEnabled} disabled={save.isPending} onChange={value => updateReportScope('enabled', value)} aria-label={`${selectedDepartment?.name} 承担周通报`} /><span>{reportEnabled ? '承担' : '未承担'}</span></td>
              <td className="department-defaults-action-summary">{reportSummary}</td>
              <td className="department-defaults-role-summary">{draft.reportRoles.map(role => <div key={role.roleCode} className="department-defaults-role-line"><span>{reportRoleName(role.roleCode)}</span><span>{reportEnabled ? enabledReportActions(draft.reportScope, role) : '无'}</span></div>)}</td>
              <td><div className="department-defaults-row-actions"><Button type="link" onClick={() => setEditor({ kind: 'reportScope' })}>部门功能</Button>
                <Button type="link" onClick={() => setEditor({ kind: 'reportRoles' })}>角色权限</Button></div></td></tr></tbody>
          </table></div>
        </section>
      </div>
      <Drawer title={editor?.kind === 'scope' ? `${editingType?.name ?? '检查项'} · 部门功能` : editor?.kind === 'inspectionRoles' ? `${editingType?.name ?? '检查项'} · 角色权限` : editor?.kind === 'reportScope' ? '周通报 · 部门功能' : '周通报 · 角色权限'}
        open={editor !== null} onClose={() => setEditor(null)} width="min(640px, 100vw)" destroyOnHidden>
        {editor?.kind === 'scope' && editingScope && <><Typography.Paragraph type="secondary">部门承担状态在列表中设置；这里选择该检查项开放的动作。</Typography.Paragraph>
          <div className="department-defaults-option-list">{inspectionActions.map(([key, label]) =>
            <div className="department-defaults-option" key={key}><span>{label}</span><Switch checked={editingScope[key]} disabled={save.isPending}
              onChange={value => updateScope(editingScope, key, value)} aria-label={`${selectedDepartment?.name} ${editingType?.name} ${label}`} /></div>)}</div>
          </>}
        {editor?.kind === 'inspectionRoles' && <><Typography.Paragraph type="secondary">设置 {editingType?.name} 的岗位动作；部长与副部长共用一列。先在列表开启部门承担，再开放部门动作。</Typography.Paragraph>
          <div className="department-defaults-table-scroll"><table className="department-defaults-table department-defaults-matrix"><thead><tr><th scope="col">业务动作</th><th scope="col">部门</th>{editingInspectionRoles.map(role => <th scope="col" key={role.roleCode}>{roleName(role.roleCode)}</th>)}</tr></thead>
            <tbody>{inspectionActions.map(([key, label]) => <tr key={key}><th scope="row">{label}</th><td>{!editingScope?.enabled ? '未承担' : editingScope[key] ? '已开放' : '未开放'}</td>{editingInspectionRoles.map(role => <td key={role.roleCode}><Switch size="small" checked={role[key]}
              disabled={save.isPending || !editingScope?.enabled || !editingScope[key]}
              onChange={value => updateInspectionRole(role, key, value)} aria-label={`${selectedDepartment?.name} ${editingType?.name} ${roleName(role.roleCode)} ${label}`} /></td>)}</tr>)}</tbody></table></div>
          <p className="department-defaults-footnote">部门未承担或未开放同一动作时，岗位开关不可操作。此前已保存的岗位配置仍保留，但不会生效。检查 Excel 导出尚未交付。</p></>}
        {editor?.kind === 'reportScope' && <><Typography.Paragraph type="secondary">部门承担状态在列表中设置；这里选择周通报开放的动作。</Typography.Paragraph>
          <div className="department-defaults-option-list">{reportActions.map(([key, label]) =>
            <div className="department-defaults-option" key={key}><span>{label}</span><Switch checked={draft.reportScope[key]} disabled={save.isPending}
              onChange={value => updateReportScope(key, value)} aria-label={`${selectedDepartment?.name} 周通报 ${label}`} /></div>)}</div></>}
        {editor?.kind === 'reportRoles' && <><Typography.Paragraph type="secondary">先在列表开启部门承担，再开放对应部门动作。委员不能审核、退回、定稿或修改文件输出设置。</Typography.Paragraph>
          <div className="department-defaults-table-scroll"><table className="department-defaults-table department-defaults-matrix"><thead><tr><th scope="col">业务动作</th><th scope="col">部门</th>{draft.reportRoles.map(role => <th scope="col" key={role.roleCode}>{reportRoleName(role.roleCode)}</th>)}</tr></thead>
            <tbody>{reportActions.map(([key, label]) => <tr key={key}><th scope="row">{label}</th><td>{!draft.reportScope.enabled ? '未承担' : draft.reportScope[key] ? '已开放' : '未开放'}</td>{draft.reportRoles.map(role => <td key={role.roleCode}><Switch size="small" checked={role[key]}
              disabled={save.isPending || !draft.reportScope.enabled || !draft.reportScope[key] || (role.roleCode === 'MEMBER' && (key === 'canReview' || key === 'canReturn' || key === 'canFinalize' || key === 'canOutputSettings'))}
              onChange={value => updateReportRole(role, key, value)} aria-label={`${selectedDepartment?.name} ${reportRoleName(role.roleCode)} ${label}`} /></td>)}</tr>)}</tbody></table></div>
          <p className="department-defaults-footnote">部门未承担或未开放同一动作时，岗位开关不可操作。此前已保存的岗位配置仍保留，但不会生效。发布渠道尚未交付。</p></>}
      </Drawer>
    </>}
  </div>
}
