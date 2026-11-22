import { Alert, Button, Card, Empty, Form, Input, InputNumber, Modal, Select, Space, Steps, Switch, Table, Tag, Typography, message } from 'antd'
import { BookOutlined, ClearOutlined, ClockCircleOutlined, DragOutlined, ProfileOutlined } from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { useEffect, useMemo, useState } from 'react'
import { ApiError } from '../../shared/api/httpClient'
import { getRegistrationOptions, getDisciplineLevels } from '../../shared/api/adminApi'
import { calculateRuleExample, orderGroupsByInitialConfiguration, type DisciplineGroup, type DisciplineRuleVersion, type DisciplineTerm, type DisciplineTier, type DisciplineValidationIssue } from '../../features/discipline-config/model'
import { createDisciplineRuleDraft, getCurrentDisciplineRule, getDisciplineRuleVersions, publishDisciplineRuleDraft, saveDisciplineRuleDraft, validateDisciplineRuleDraft } from '../../features/discipline-config/api'
import './DisciplineConfigPage.css'

const stepTitles = ['归组并命名', '确认计量与换算', '配置处分档位', '检查并发布']
type PendingTransition = { kind: 'version'; id?: string } | { kind: 'route'; path: string } | { kind: 'history' | 'editor' }
type GroupUndoSnapshot = { groups: DisciplineGroup[]; formulaNeedsReview: Set<string>; tierNeedsReview: Set<string> }
type TierTextEdit = { groupKey: string; code: string; field: 'name' | 'clause'; value: string }

const dateText = (value?: string) => value
  ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
  : '—'

export function DisciplineConfigPage({ mode = 'edit', canEdit, canPublish }: { mode?: 'view' | 'edit'; canEdit: boolean; canPublish: boolean }) {
  const client = useQueryClient()
  const navigate = useNavigate()
  const [messageApi, contextHolder] = message.useMessage()
  const [workspace, setWorkspace] = useState<'editor' | 'history'>('editor')
  const [viewId, setViewId] = useState<string>()
  const [editing, setEditing] = useState<DisciplineRuleVersion>()
  const [createdDraft, setCreatedDraft] = useState<DisciplineRuleVersion>()
  const [publishTarget, setPublishTarget] = useState<DisciplineRuleVersion>()
  const [step, setStep] = useState(0)
  const [selectedGroupKey, setSelectedGroupKey] = useState<string>()
  const [selectedTypeId, setSelectedTypeId] = useState<string>()
  const [targetGroupKey, setTargetGroupKey] = useState<string>()
  const [sampleValues, setSampleValues] = useState<Record<string, number>>({})
  const [validationIssues, setValidationIssues] = useState<DisciplineValidationIssue[]>([])
  const [validating, setValidating] = useState(false)
  const [dragOverKey, setDragOverKey] = useState<string>()
  const [undoGroups, setUndoGroups] = useState<GroupUndoSnapshot[]>([])
  const [formulaNeedsReview, setFormulaNeedsReview] = useState<Set<string>>(new Set())
  const [tierNeedsReview, setTierNeedsReview] = useState<Set<string>>(new Set())
  const [lastSavedAt, setLastSavedAt] = useState<string>()
  const [pendingTransition, setPendingTransition] = useState<PendingTransition>()
  const [publishConfirmOpen, setPublishConfirmOpen] = useState(false)
  const [tierBoundaryPrompt, setTierBoundaryPrompt] = useState<{ groupKey: string; code: string; name: string; minimum: number }>()
  const [tierBoundaryValue, setTierBoundaryValue] = useState<number>()
  const [tierTextEdit, setTierTextEdit] = useState<TierTextEdit>()

  const current = useQuery({ queryKey: ['discipline-config', 'current'], queryFn: getCurrentDisciplineRule })
  const versions = useQuery({ queryKey: ['discipline-config', 'versions'], queryFn: getDisciplineRuleVersions })
  const catalogs = useQuery({ queryKey: ['discipline-config', 'entry-options'], queryFn: getRegistrationOptions, enabled: mode === 'edit' })
  const levels = useQuery({ queryKey: ['discipline-config', 'levels'], queryFn: getDisciplineLevels, enabled: mode === 'edit' })
  const tierDictionary = levels.data?.types.find(type => type.code === 'DISCIPLINE_LEVEL')
  const tierOptions = levels.data?.items.filter(item => item.enabled) ?? []
  const draftInHistory = versions.data?.items.find((item) => item.status === 'DRAFT')
  const draftVersion = createdDraft ?? draftInHistory
  const activeVersion = viewId
    ? versions.data?.items.find((item) => item.id === viewId)
    : mode === 'edit' ? draftVersion ?? current.data?.current ?? undefined : current.data?.current ?? undefined
  const version = editing?.id === activeVersion?.id ? editing : activeVersion
  const dirty = Boolean(editing && version?.status === 'DRAFT')
  const editable = mode === 'edit' && version?.status === 'DRAFT' && workspace === 'editor' && !viewId
  const groups = version?.groups ?? []
  const selectedGroup = groups.find((item) => item.key === selectedGroupKey) ?? groups[0]
  const knownTypes = useMemo(() => (catalogs.data?.inspectionTypes ?? []).filter((type) => type.enabled), [catalogs.data])
  const membershipCounts = groups.flatMap((group) => group.terms).reduce((counts, term) => counts.set(term.inspectionTypeId, (counts.get(term.inspectionTypeId) ?? 0) + 1), new Map<string, number>())
  const exactlyAssignedCount = knownTypes.filter((type) => membershipCounts.get(type.id) === 1).length
  const groupForType = (typeId: string) => groups.find((group) => group.terms.some((term) => term.inspectionTypeId === typeId))
  const sourceUnitForType = (typeId: string) => knownTypes.find((type) => type.id === typeId)?.code === 'ATTENDANCE_RATE' ? '学时' : '次'
  const iconForGroup = (group: DisciplineGroup) => {
    const kinds = group.terms.map((term) => catalogs.data?.inspectionTypes.find((type) => type.id === term.inspectionTypeId)?.extensionKind)
    if (kinds.length === 0 || kinds.some((kind) => !kind)) return <ProfileOutlined aria-hidden="true" />
    if (kinds.every((kind) => kind?.endsWith('_HYGIENE'))) return <ClearOutlined aria-hidden="true" />
    if (kinds.every((kind) => kind === 'COURSE_ATTENDANCE')) return <BookOutlined aria-hidden="true" />
    if (kinds.every((kind) => ['MORNING_ATTENDANCE', 'EVENING_SELF_STUDY', 'LATE_RETURN'].includes(kind ?? ''))) return <ClockCircleOutlined aria-hidden="true" />
    return <ProfileOutlined aria-hidden="true" />
  }
  const fieldControlId = (path: string) => `discipline-${path.replace(/[^a-zA-Z0-9_-]/g, '-')}`

  useEffect(() => {
    const warnBeforeLeave = (event: BeforeUnloadEvent) => {
      if (!dirty) return
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', warnBeforeLeave)
    const warnBeforeRouteLeave = (event: MouseEvent) => {
      if (!dirty || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
      const target = event.target instanceof Element ? event.target.closest('a[href]') : null
      if (!(target instanceof HTMLAnchorElement) || target.target === '_blank') return
      const destination = new URL(target.href, window.location.href)
      if (destination.origin !== window.location.origin || destination.pathname === window.location.pathname) return
      event.preventDefault()
      event.stopPropagation()
      setPendingTransition({ kind: 'route', path: `${destination.pathname}${destination.search}${destination.hash}` })
    }
    document.addEventListener('click', warnBeforeRouteLeave, true)
    return () => {
      window.removeEventListener('beforeunload', warnBeforeLeave)
      document.removeEventListener('click', warnBeforeRouteLeave, true)
    }
  }, [dirty, navigate])

  const refresh = async () => Promise.all([
    client.invalidateQueries({ queryKey: ['discipline-config', 'current'] }),
    client.invalidateQueries({ queryKey: ['discipline-config', 'versions'] }),
  ])
  const createDraft = useMutation({
    mutationFn: createDisciplineRuleDraft,
    onSuccess: async (created) => {
      setCreatedDraft(created)
      setViewId(undefined)
      setWorkspace('editor')
      setStep(0)
      setEditing(undefined)
      await refresh()
      navigate('/discipline/config/edit')
      messageApi.success('规则草稿已创建')
    },
    onError: () => messageApi.error('创建失败，请重试'),
  })
  const saveDraft = useMutation({
    mutationFn: saveDisciplineRuleDraft,
    onSuccess: async (saved) => {
      setCreatedDraft(saved)
      setLastSavedAt(new Date().toISOString())
      setEditing(undefined)
      await refresh()
      messageApi.success('规则草稿已保存')
    },
    onError: (error) => {
      if (error instanceof ApiError) setValidationIssues(error.details)
      messageApi.error('保存失败，输入内容已保留；请按字段提示修正后重试')
    },
  })
  const publishRule = useMutation({
    mutationFn: (input: { id: string; rowVersion: number }) => publishDisciplineRuleDraft(input.id, input.rowVersion),
    onSuccess: async () => {
      setEditing(undefined)
      setCreatedDraft(undefined)
      setPublishTarget(undefined)
      setPublishConfirmOpen(false)
      await refresh()
      messageApi.success('默认规则版本已发布')
      navigate('/discipline/config', { replace: true })
    },
    onError: (error) => {
      if (error instanceof ApiError) setValidationIssues(error.details)
      messageApi.error('发布失败，当前规则未更改；请检查字段错误或刷新后重试')
    },
  })

  const issueAt = (path: string) => validationIssues.find((issue) => issue.path === path)
  const validationStepForPath = (path: string) => path.includes('.tiers') ? 2 : path.includes('.terms') || path.endsWith('.unit') ? 1 : 0
  const stepHasValidationIssue = (index: number) => validationIssues.some((issue) => validationStepForPath(issue.path) === index)
    || (index === 1 && formulaNeedsReview.size > 0)
    || (index === 2 && tierNeedsReview.size > 0)
  const fieldError = (path: string) => {
    const issue = issueAt(path)
    return issue ? { validateStatus: 'error' as const, help: issue.message } : {}
  }
  const issueLocation = (issue: DisciplineValidationIssue) => {
    const groupIndex = Number(issue.path.match(/groups\[(\d+)\]/)?.[1] ?? -1)
    const tierIndex = Number(issue.path.match(/tiers\[(\d+)\]/)?.[1] ?? -1)
    const field = issue.path.split('.').at(-1)
    const fieldNames: Record<string, string> = { name: '处分名称', lower: '起始值', upper: '结束值', clause: '依据条款', teacherConfirmationRequired: '人工确认方式', tiers: '处分档位', terms: '检查项归属', unit: '累计单位', factor: '换算系数', operator: '运算方式' }
    const groupName = groups[groupIndex]?.name || '累计项'
    const tierName = groups[groupIndex]?.tiers[tierIndex]?.name
    return `${groupName}${tierName ? ` · ${tierName}` : ''} · ${fieldNames[field ?? ''] ?? '配置项'}：${issue.message}`
  }
  const stepErrorMessage = (index: number, issues: DisciplineValidationIssue[]) => {
    const details = issues.slice(0, 2).map(issueLocation)
    const remaining = issues.length - details.length
    return `第 ${index + 1} 步“${stepTitles[index]}”未通过${details.length ? `：${details.join('；')}` : ''}${remaining > 0 ? `；另有 ${remaining} 项` : ''}。请留在本步骤修正标红字段后再继续。`
  }
  const clearValidationIssues = (paths: string[]) => {
    const clearing = new Set(paths)
    for (const path of paths) {
      const upper = path.match(/^(groups\[\d+\]\.tiers\[(\d+)\])\.upper$/)
      const lower = path.match(/^(groups\[\d+\]\.tiers\[(\d+)\])\.lower$/)
      if (upper) clearing.add(`${upper[1].replace(/tiers\[\d+\]$/, `tiers[${Number(upper[2]) + 1}]`)}.lower`)
      if (lower && Number(lower[2]) > 0) clearing.add(path.replace(/tiers\[\d+\]\.lower$/, `tiers[${Number(lower[2]) - 1}].upper`))
    }
    setValidationIssues((issues) => issues.filter((issue) => ![...clearing].some((path) => issue.path === path || issue.path.startsWith(`${path}.`) || issue.path.startsWith(`${path}[`))))
  }
  const setGroups = (nextGroups: DisciplineGroup[], clearPaths: string[] = []) => {
    clearValidationIssues(clearPaths)
    if (version) setEditing({ ...version, groups: nextGroups })
  }
  const updateGroup = (key: string, patch: Partial<DisciplineGroup>, clearPaths: string[] = []) => {
    setGroups(groups.map((group) => group.key === key ? { ...group, ...patch } : group), clearPaths)
  }
  const updateTerm = (id: string, patch: Partial<DisciplineTerm>, clearPaths: string[] = []) => {
    setGroups(groups.map((group) => ({
      ...group,
      terms: group.terms.map((term) => term.inspectionTypeId === id ? { ...term, ...patch } : term),
    })), clearPaths)
  }

  const validateStep = (stepIndex: number): DisciplineValidationIssue[] => {
    const issues: DisciplineValidationIssue[] = []
    const add = (path: string, code: string, message: string) => issues.push({ path, code, message })

    if (stepIndex === 0) {
      const names = new Set<string>()
      groups.forEach((group, groupIndex) => {
        const name = group.name.trim()
        if (!name || names.has(name)) add(`groups[${groupIndex}].name`, 'GROUP_NAME_REQUIRED', '累计组名称不能为空且不能重复')
        names.add(name)
        if (group.terms.length === 0) add(`groups[${groupIndex}].terms`, 'GROUP_MEMBER_REQUIRED', '累计组至少需要一个检查项')
      })
      if (groups.length === 0 || knownTypes.length === 0 || knownTypes.some((type) => membershipCounts.get(type.id) !== 1)) {
        add('groups', 'INSPECTION_TYPE_UNASSIGNED', '每个启用的检查项都必须且只能归入一个累计项')
      }
    }

    if (stepIndex === 1) {
      groups.forEach((group, groupIndex) => {
        if (!group.unit.trim()) add(`groups[${groupIndex}].unit`, 'RESULT_UNIT_REQUIRED', '请填写累计组的统一单位')
        group.terms.forEach((term, termIndex) => {
          if (!Number.isSafeInteger(term.factor) || term.factor <= 0 || term.factor > 999999) {
            add(`groups[${groupIndex}].terms[${termIndex}].factor`, 'INVALID_CONVERSION_FACTOR', '换算系数必须为1至999999之间的正整数')
          }
          if ((term.operator !== '+' && term.operator !== '-') || (termIndex === 0 && term.operator !== '+')) {
            add(`groups[${groupIndex}].terms[${termIndex}].operator`, 'INVALID_OPERATOR', '首项使用加法，其他项请选择加或减')
          }
        })
      })
    }

    if (stepIndex === 2) {
      if (tierOptions.length === 0) add('groups[0].tiers', 'TIER_ORDER_INVALID', '处分档位字典暂无启用项，请检查字典配置')
      const orderByCode = new Map(tierOptions.map((option, index) => [option.value, index]))
      groups.forEach((group, groupIndex) => {
        if (group.tiers.length === 0) {
          add(`groups[${groupIndex}].tiers`, 'TIER_REQUIRED', '至少保留一个处分档位')
          return
        }

        let previousOrder = -1
        const names = new Set<string>()
        let previousUpper: number | undefined
        group.tiers.forEach((tier, tierIndex) => {
          const tierPath = `groups[${groupIndex}].tiers[${tierIndex}]`
          const order = orderByCode.get(tier.code)
          if (order === undefined || order <= previousOrder) add(`${tierPath}.code`, 'TIER_ORDER_INVALID', '处分档位须来自启用字典并按档位顺序排列，不能重复')
          if (order !== undefined) previousOrder = order

          const name = tier.name.trim()
          if (!name || names.has(name)) add(`${tierPath}.name`, 'TIER_NAME_INVALID', '处分名称不能为空且同一累计项内不能重复')
          names.add(name)
          if (!tier.clause.trim()) add(`${tierPath}.clause`, 'CLAUSE_REQUIRED', '请填写对应处分依据条款')

          const validBound = (value: number) => Number.isSafeInteger(value) && value >= 0 && value <= 99999999
          if (!validBound(tier.lower)) add(`${tierPath}.lower`, 'INVALID_TIER_BOUND', '起始值必须为0至99999999之间的整数')
          if (tierIndex > 0 && previousUpper !== undefined && tier.lower !== previousUpper) add(`${tierPath}.lower`, 'TIER_GAP_OR_OVERLAP', '处分区间存在空档或重叠')

          const isLast = tierIndex === group.tiers.length - 1
          if (tier.upper !== undefined && (!validBound(tier.upper) || tier.upper <= tier.lower)) add(`${tierPath}.upper`, 'INVALID_TIER_BOUND', '结束值必须为大于起始值且不超过99999999的整数')
          if (!isLast && tier.upper === undefined) add(`${tierPath}.upper`, 'TIER_UPPER_REQUIRED', '仅最高档位的结束值可以留空')
          if (isLast && tier.upper !== undefined) add(`${tierPath}.upper`, 'TOP_TIER_MUST_BE_OPEN', '最高档位的结束值应留空')
          previousUpper = tier.upper
        })
      })
    }

    return issues
  }
  const changeStep = (nextStep: number, groupKey?: string) => {
    if (nextStep === step || nextStep < 0 || nextStep > 3) return
    if (nextStep > step) {
      for (let checkedStep = step; checkedStep < nextStep; checkedStep += 1) {
        if (checkedStep === 2 && levels.isLoading) {
          messageApi.warning('处分档位字典正在加载，请稍后再继续。')
          return
        }
        const stepIssues = validateStep(checkedStep)
        setValidationIssues((current) => [
          ...current.filter((issue) => validationStepForPath(issue.path) !== checkedStep),
          ...stepIssues,
        ])
        if (stepIssues.length > 0) {
          messageApi.warning(stepErrorMessage(checkedStep, stepIssues))
          return
        }
        if (checkedStep === 1 && formulaNeedsReview.size > 0) {
          messageApi.warning('请先核对已变化的累计项换算公式，再继续。')
          return
        }
        if (checkedStep === 2 && tierNeedsReview.size > 0) {
          messageApi.warning('请先核对已变化的累计项处分档位，再继续。')
          return
        }
      }
    }
    if (groupKey) setSelectedGroupKey(groupKey)
    setStep(nextStep)
  }
  const transition = (target: PendingTransition) => {
    if (dirty) setPendingTransition(target)
    else applyTransition(target)
  }
  const applyTransition = (target: PendingTransition) => {
    setPendingTransition(undefined)
    if (target.kind === 'version') { setViewId(target.id); setWorkspace('history') }
    if (target.kind === 'history') { setWorkspace('history'); setViewId(undefined) }
    if (target.kind === 'editor') { setWorkspace('editor'); setViewId(undefined) }
    if (target.kind === 'route') navigate(target.path)
  }
  const saveAndTransition = async () => {
    if (!version || !dirty) return
    try {
      await saveDraft.mutateAsync(version)
      if (pendingTransition) applyTransition(pendingTransition)
    } catch { /* Mutation shows errors and retains the local draft. */ }
  }
  const discardAndTransition = () => {
    if (pendingTransition) applyTransition(pendingTransition)
    setEditing(undefined)
    setValidationIssues([])
  }

  const rememberGroupChange = () => setUndoGroups((undo) => [...undo.slice(-9), { groups, formulaNeedsReview: new Set(formulaNeedsReview), tierNeedsReview: new Set(tierNeedsReview) }])
  const termForType = (typeId: string): DisciplineTerm | undefined => {
    const existing = groups.flatMap((group) => group.terms).find((term) => term.inspectionTypeId === typeId)
    if (existing) return existing
    const source = knownTypes.find((type) => type.id === typeId)
    return source ? { inspectionTypeId: source.id, inspectionTypeName: source.name, sourceUnit: source.code === 'ATTENDANCE_RATE' ? '学时' : '次', operator: '+', factor: 1 } : undefined
  }
  const moveType = (typeId: string, targetKey: string) => {
    const fromIndex = groups.findIndex((group) => group.terms.some((term) => term.inspectionTypeId === typeId))
    const toIndex = groups.findIndex((group) => group.key === targetKey)
    if (toIndex < 0 || fromIndex === toIndex) return
    const term = termForType(typeId)
    if (!term) return
    rememberGroupChange()
    const next = groups.map((group, index) => index === fromIndex
      ? { ...group, terms: group.terms.filter((item) => item.inspectionTypeId !== typeId) }
      : index === toIndex
        ? { ...group, terms: [...group.terms, { ...term, operator: '+' as const }] }
        : group)
    const affected = new Set([fromIndex >= 0 ? groups[fromIndex].key : undefined, targetKey].filter((key): key is string => Boolean(key)))
    setFormulaNeedsReview((current) => new Set([...current, ...affected]))
    setTierNeedsReview((current) => new Set([...current, ...affected]))
    setGroups(next, [`groups[${fromIndex}].terms`, `groups[${toIndex}].terms`, 'groups'])
    setSelectedGroupKey(targetKey)
    messageApi.success({ content: `已将“${term.inspectionTypeName}”移入“${groups[toIndex].name || '待命名累计项'}”`, duration: 2 })
  }
  const createGroupWithType = (typeId: string) => {
    if (!version) return
    const type = knownTypes.find((item) => item.id === typeId)
    const term = termForType(typeId)
    if (!type || !term) return
    rememberGroupChange()
    const newGroupKey = `group-${crypto.randomUUID()}`
    const sourceGroup = groupForType(typeId)
    const next = groups
      .map((group) => group.key === sourceGroup?.key ? { ...group, terms: group.terms.filter((item) => item.inspectionTypeId !== typeId) } : group)
      .concat({ key: newGroupKey, name: type.name, unit: '', terms: [{ ...term, operator: '+' as const }], tiers: [] })
    if (sourceGroup) {
      setFormulaNeedsReview((current) => new Set(current).add(sourceGroup.key).add(newGroupKey))
      setTierNeedsReview((current) => new Set(current).add(sourceGroup.key).add(newGroupKey))
    } else {
      setFormulaNeedsReview((current) => new Set(current).add(newGroupKey))
      setTierNeedsReview((current) => new Set(current).add(newGroupKey))
    }
    setGroups(next, ['groups'])
    setSelectedGroupKey(newGroupKey)
    messageApi.success(`已新建“${type.name}”累计项，可在行内修改名称`)
  }
  const moveToUnassigned = (typeId: string) => {
    const fromIndex = groups.findIndex((group) => group.terms.some((term) => term.inspectionTypeId === typeId))
    if (fromIndex < 0) return
    rememberGroupChange()
    const source = groups[fromIndex]
    const next = groups.map((group) => group.key === source.key
      ? { ...group, terms: group.terms.filter((item) => item.inspectionTypeId !== typeId) }
      : group)
    setFormulaNeedsReview((current) => new Set(current).add(source.key))
    setTierNeedsReview((current) => new Set(current).add(source.key))
    setGroups(next, [`groups[${fromIndex}].terms`])
    messageApi.success('检查项已移回未归组列表，可撤销此操作')
  }
  const deleteGroup = (groupKey: string) => {
    const removed = groups.find((group) => group.key === groupKey)
    if (!removed) return
    rememberGroupChange()
    setGroups(groups.filter((group) => group.key !== groupKey), ['groups'])
    setFormulaNeedsReview((current) => { const next = new Set(current); next.delete(groupKey); return next })
    setTierNeedsReview((current) => { const next = new Set(current); next.delete(groupKey); return next })
    if (selectedGroupKey === groupKey) setSelectedGroupKey(groups.find((group) => group.key !== groupKey)?.key)
    if (targetGroupKey === groupKey) setTargetGroupKey(undefined)
    messageApi.success(`已删除累计项“${removed.name}”，其检查项已返回未归组列表`)
  }
  const undoGroupChange = () => {
    const previous = undoGroups.at(-1)
    if (!previous) return
    setUndoGroups((undo) => undo.slice(0, -1))
    setFormulaNeedsReview(previous.formulaNeedsReview)
    setTierNeedsReview(previous.tierNeedsReview)
    setGroups(previous.groups, ['groups'])
    messageApi.success('已撤销最近一次归组调整')
  }
  const setGroupField = (group: DisciplineGroup, field: 'name'|'unit'|'violationName', value: string) => {
    const groupIndex = groups.findIndex((item) => item.key === group.key)
    const patch = { [field]:value }
    setUndoGroups([])
    updateGroup(group.key, patch, [`groups[${groupIndex}].${field}`])
    if (field === 'unit') {
      setFormulaNeedsReview((current) => new Set(current).add(group.key))
      setTierNeedsReview((current) => new Set(current).add(group.key))
    }
  }
  const setTierField = (group: DisciplineGroup, tierIndex: number, patch: Partial<DisciplineTier>, changed: 'authorityNumber'|'clauseReference'|'name' | 'lower' | 'upper' | 'clause' | 'teacherConfirmationRequired') => {
    const groupIndex = groups.findIndex((item) => item.key === group.key)
    const prefix = `groups[${groupIndex}].tiers[${tierIndex}]`
    const related = [`${prefix}.${changed}`]
    if (changed === 'upper' && tierIndex < group.tiers.length - 1) related.push(`groups[${groupIndex}].tiers[${tierIndex + 1}].lower`)
    if (changed === 'lower' && tierIndex > 0) related.push(`groups[${groupIndex}].tiers[${tierIndex - 1}].upper`)
    setUndoGroups([])
    updateGroup(group.key, { tiers: group.tiers.map((tier, index) => index === tierIndex ? { ...tier, ...patch } : tier) }, related)
  }
  const setTermField = (group: DisciplineGroup, term: DisciplineTerm, patch: Partial<DisciplineTerm>, field: keyof DisciplineTerm) => {
    const groupIndex = groups.findIndex((item) => item.key === group.key)
    const termIndex = group.terms.findIndex((item) => item.inspectionTypeId === term.inspectionTypeId)
    setUndoGroups([])
    updateTerm(term.inspectionTypeId, patch, [`groups[${groupIndex}].terms[${termIndex}].${field}`])
    setFormulaNeedsReview((current) => new Set(current).add(group.key))
    setTierNeedsReview((current) => new Set(current).add(group.key))
  }
  const confirmFormulaReview = (groupKey: string) => setFormulaNeedsReview((current) => { const next = new Set(current); next.delete(groupKey); return next })
  const confirmTierReview = (groupKey: string) => setTierNeedsReview((current) => { const next = new Set(current); next.delete(groupKey); return next })

  const validateAndPreparePublish = async () => {
    if (!version) return
    try {
      setValidating(true)
      const target = dirty ? await saveDraft.mutateAsync(version) : version
      await validateDisciplineRuleDraft(target.id)
      setValidationIssues([])
      setEditing(undefined)
      setCreatedDraft(target)
      setPublishTarget(target)
      await refresh()
      setPublishConfirmOpen(true)
    } catch (error) {
      if (error instanceof ApiError) {
        setValidationIssues(error.details)
        const affectedSteps = [...new Set(error.details.map((issue) => validationStepForPath(issue.path)))].sort((a, b) => a - b)
        const firstStep = affectedSteps[0]
        const details = error.details.slice(0, 2).map(issueLocation)
        const remaining = error.details.length - details.length
        const errorDetail = details.length ? `（${details.join('；')}${remaining > 0 ? `；另有 ${remaining} 项` : ''}）` : '，服务端未返回具体字段位置'
        messageApi.error(`规则校验未通过${firstStep === undefined ? '' : `：第 ${firstStep + 1} 步“${stepTitles[firstStep]}”`}发现 ${error.details.length} 项需要修改${errorDetail}。请从顶部步骤进入对应步骤，按字段提示修正后重新校验。`)
      } else messageApi.error('保存或校验失败，草稿内容仍保留，请重试。')
    } finally {
      setValidating(false)
    }
  }

  const addTier = (group: DisciplineGroup) => {
    const dictionaryItem = tierOptions.find((item) => !group.tiers.some((tier) => tier.code === item.value))
    if (!dictionaryItem) return
    const order = new Map(tierOptions.map((item, index) => [item.value, index]))
    const nextTiers = [...group.tiers, { code: dictionaryItem.value, name: dictionaryItem.label, teacherConfirmationRequired: dictionaryItem.value !== 'NOTICE', lower: 0, clause: '' }]
      .sort((left, right) => (order.get(left.code) ?? Number.MAX_SAFE_INTEGER) - (order.get(right.code) ?? Number.MAX_SAFE_INTEGER))
    const index = nextTiers.findIndex((tier) => tier.code === dictionaryItem.value)
    const following = nextTiers[index + 1]
    const previous = nextTiers[index - 1]
    if (!following && previous && previous.upper === undefined) {
      setTierBoundaryPrompt({ groupKey: group.key, code: dictionaryItem.value, name: dictionaryItem.label, minimum: previous.lower })
      setTierBoundaryValue(undefined)
      return
    }
    if (following && previous?.upper !== undefined && previous.lower < previous.upper) {
      const split = Math.floor(previous.lower + (previous.upper - previous.lower) / 2)
      if (split <= previous.lower || split >= previous.upper || split >= following.lower) {
        messageApi.warning('相邻整数阈值不足以拆分此区间，请先调整区间后再添加档位。')
        return
      }
      nextTiers[index - 1] = { ...previous, upper: split }
      nextTiers[index] = { ...nextTiers[index], lower: split, upper: following.lower }
    } else if (!previous) {
      nextTiers[index] = { ...nextTiers[index], lower: 0, upper: following?.lower }
    } else {
      nextTiers[index] = { ...nextTiers[index], lower: previous.upper ?? previous.lower }
    }
    updateGroup(group.key, { tiers: nextTiers }, [`groups[${groups.findIndex((item) => item.key === group.key)}].tiers`])
    setUndoGroups([])
    setTierNeedsReview((current) => new Set(current).add(group.key))
  }
  const confirmTierBoundary = () => {
    const prompt = tierBoundaryPrompt
    const group = groups.find((item) => item.key === prompt?.groupKey)
    if (!prompt || !group || tierBoundaryValue === undefined || !Number.isSafeInteger(tierBoundaryValue) || tierBoundaryValue <= prompt.minimum) {
      messageApi.warning('新档起始值必须为大于当前最高档起始值的整数')
      return
    }
    const order = new Map(tierOptions.map((item, index) => [item.value, index]))
    const tiers = [...group.tiers, { code: prompt.code, name: prompt.name, teacherConfirmationRequired: prompt.code !== 'NOTICE', lower: tierBoundaryValue, clause: '' }]
      .sort((left, right) => (order.get(left.code) ?? Number.MAX_SAFE_INTEGER) - (order.get(right.code) ?? Number.MAX_SAFE_INTEGER))
    const index = tiers.findIndex((tier) => tier.code === prompt.code)
    if (index > 0) tiers[index - 1] = { ...tiers[index - 1], upper: tierBoundaryValue }
    if (index < tiers.length - 1) tiers[index] = { ...tiers[index], upper: tiers[index + 1].lower }
    updateGroup(group.key, { tiers }, [`groups[${groups.findIndex((item) => item.key === group.key)}].tiers`])
    setUndoGroups([])
    setTierNeedsReview((current) => new Set(current).add(group.key))
    setTierBoundaryPrompt(undefined)
    setTierBoundaryValue(undefined)
  }
  const saveTierText = (group: DisciplineGroup, index: number) => {
    if (!tierTextEdit || !tierTextEdit.value.trim()) {
      messageApi.warning(tierTextEdit?.field === 'name' ? '处分名称不能为空，请填写后保存。' : '依据条款不能为空，请填写后保存。')
      return
    }
    setTierField(group, index, { [tierTextEdit.field]: tierTextEdit.value.trim() }, tierTextEdit.field)
    setTierTextEdit(undefined)
  }
  const removeTier = (group: DisciplineGroup, tierIndex: number) => {
    const removed = group.tiers[tierIndex]
    const tiers = group.tiers.filter((_, index) => index !== tierIndex)
    if (tierIndex > 0 && removed?.upper !== undefined) tiers[tierIndex - 1] = { ...tiers[tierIndex - 1], upper: removed.upper }
    if (tiers.length > 0) {
      for (let index = 1; index < tiers.length; index += 1) tiers[index] = { ...tiers[index], lower: tiers[index - 1].upper ?? tiers[index].lower }
      tiers[tiers.length - 1] = { ...tiers[tiers.length - 1], upper: undefined }
    }
    updateGroup(group.key, { tiers }, [`groups[${groups.findIndex((item) => item.key === group.key)}].tiers`])
    setUndoGroups([])
    setTierNeedsReview((current) => new Set(current).add(group.key))
  }
  const summarizeDiff = (item: DisciplineRuleVersion) => {
    const previous = versions.data?.items.filter((candidate) => candidate.version < item.version).sort((a, b) => b.version - a.version)[0]
    if (!previous) return '首个版本'
    const before = new Map(previous.groups.map((group) => [group.key, group]))
    const after = new Map(item.groups.map((group) => [group.key, group]))
    const added = [...after.keys()].filter((key) => !before.has(key)).length
    const removed = [...before.keys()].filter((key) => !after.has(key)).length
    const changed = [...after.keys()].filter((key) => before.has(key) && JSON.stringify(after.get(key)) !== JSON.stringify(before.get(key))).length
    return `新增 ${added} 组 · 移除 ${removed} 组 · 调整 ${changed} 组`
  }

  const groupColumns = [
    { title: '累计项名称', dataIndex: 'name', width: 220, render: (name: string, group: DisciplineGroup) => {
      const path = `groups[${groups.findIndex((item) => item.key === group.key)}].name`
      return <Form.Item className="discipline-group-name-item" {...fieldError(path)}><Input id={fieldControlId(path)} aria-label={`${name || '待命名'}累计项名称`} value={name} placeholder="输入累计项名称" onFocus={() => setSelectedGroupKey(group.key)} onChange={(event) => setGroupField(group, 'name', event.target.value)} disabled={!editable} /></Form.Item>
    } },
    { title: '成员检查项（拖出可移回未归组）', render: (_: unknown, group: DisciplineGroup) => group.terms.length > 0 ? <Space wrap>{group.terms.map((term) => <span className="discipline-member-chip" key={term.inspectionTypeId} draggable={editable} onDragStart={(event) => event.dataTransfer.setData('text/plain', term.inspectionTypeId)}><DragOutlined aria-hidden="true" />{knownTypes.find((type) => type.id === term.inspectionTypeId)?.name ?? term.inspectionTypeName}<Button size="small" type="text" aria-label={`移出 ${knownTypes.find((type) => type.id === term.inspectionTypeId)?.name ?? term.inspectionTypeName}`} disabled={!editable} onClick={() => moveToUnassigned(term.inspectionTypeId)}>移出</Button></span>)}</Space> : <Tag color="orange">空组，可加入检查项或删除</Tag> },
    { title: '单位', dataIndex: 'unit', width: 100, render: (unit: string) => unit || <Typography.Text type="secondary">待确认</Typography.Text> },
    { title: '操作', width: 90, render: (_: unknown, group: DisciplineGroup) => <Button danger type="text" disabled={!editable} onClick={() => deleteGroup(group.key)}>删除累计项</Button> },
  ]

  if (current.isLoading || versions.isLoading || catalogs.isLoading || levels.isLoading) return <section className="discipline-config-page"><div role="status">正在加载处分规则配置…</div></section>
  if (current.isError || versions.isError || catalogs.isError || levels.isError) {
    const forbidden = [current.error, versions.error].some((error) => error instanceof ApiError && error.status === 403)
    return <section className="discipline-config-page"><Alert type="error" showIcon title={forbidden ? '没有处分规则配置权限' : '处分规则加载失败'} description={forbidden ? '当前账号无权访问处分规则配置。' : '当前没有加载空配置，请重试。'} action={!forbidden && <Button onClick={() => { void current.refetch(); void versions.refetch(); if (mode === 'edit') { void catalogs.refetch(); void levels.refetch() } }}>重试</Button>} /></section>
  }

  const noDraft = !draftVersion
  const startEditing = () => {
    if (draftVersion) navigate('/discipline/config/edit')
    else createDraft.mutate()
  }

  return <section className={`discipline-config-page${mode === 'view' && workspace === 'editor' ? ' discipline-config-page--view' : ''}`}>
    {contextHolder}
    <header className="attendance-page-heading page-heading discipline-heading">
      <div>
        <Typography.Title level={2}>处分配置</Typography.Title>
        {current.data?.current?.publishedAt && <Typography.Text className="discipline-heading-meta">当前生效配置 · {dateText(current.data.current.publishedAt)}</Typography.Text>}
        {mode === 'edit' && lastSavedAt && <Typography.Text className="discipline-heading-meta">本次保存：{dateText(lastSavedAt)}</Typography.Text>}
      </div>
      <Space wrap className="discipline-heading-actions">
        {current.data?.current && <Tag color="green">v{current.data.current.version} · 当前生效</Tag>}
        {workspace === 'history'
        ? <Button onClick={() => {
          if (mode === 'edit') transition({ kind: 'editor' })
          else { setWorkspace('editor'); setViewId(undefined) }
        }}>返回规则配置</Button>
        : mode === 'view'
          ? <>{canEdit && <Button type="primary" loading={createDraft.isPending} onClick={startEditing}>{draftVersion ? `继续修改草稿 v${draftVersion.version}` : current.data?.current ? '修改配置' : '开始配置'}</Button>}{canPublish && draftVersion && <Button onClick={() => { setPublishTarget(draftVersion); setPublishConfirmOpen(true) }}>发布草稿 v{draftVersion.version}</Button>}<Button onClick={() => transition({ kind: 'history' })}>版本历史</Button></>
          : <>{!draftVersion && <Button type="primary" loading={createDraft.isPending} onClick={startEditing}>新建规则草稿</Button>}<Button onClick={() => transition({ kind: 'route', path: '/discipline/config' })}>配置查看</Button><Button onClick={() => transition({ kind: 'history' })}>版本历史</Button></>}
      </Space>
    </header>

    {workspace === 'history' ? <section aria-label="处分规则版本历史" className="discipline-history">
      <Card title="版本历史" extra={viewId && <Button onClick={() => transition({ kind: 'editor' })}>返回配置</Button>}>
        <Table rowKey="id" dataSource={versions.data?.items ?? []} pagination={false} scroll={{ x: 900 }} columns={[
          { title: '版本', dataIndex: 'version', render: (value: number, item: DisciplineRuleVersion) => <Button type="link" onClick={() => transition({ kind: 'version', id: item.id })}>v{value}</Button> },
          { title: '状态', render: (_: unknown, item: DisciplineRuleVersion) => item.id === current.data?.current?.id ? <Tag color="green">当前生效 · 只读</Tag> : item.status === 'DRAFT' ? <Tag color="gold">编辑中草稿</Tag> : <Tag>历史 · 只读</Tag> },
          { title: '创建人 / 时间', render: (_: unknown, item: DisciplineRuleVersion) => `${item.createdByName ?? '未知用户'} · ${dateText(item.createdAt)}` },
          { title: '发布人 / 时间', render: (_: unknown, item: DisciplineRuleVersion) => item.publishedAt ? `${item.publishedByName ?? '未知用户'} · ${dateText(item.publishedAt)}` : '尚未发布' },
          { title: '生效时间', render: (_: unknown, item: DisciplineRuleVersion) => item.id === current.data?.current?.id ? dateText(item.publishedAt) : '—' },
          { title: '相对前版变化', render: (_: unknown, item: DisciplineRuleVersion) => summarizeDiff(item) },
        ]} locale={{ emptyText: <Empty description="尚无规则版本" /> }} />
      </Card>
      {viewId && (() => {
        const selectedVersion = versions.data?.items.find((item) => item.id === viewId)
        return selectedVersion ? <Card title={`规则版本 v${selectedVersion.version} · 只读`} style={{ marginTop: 16 }}>
          <Typography.Paragraph>创建：{selectedVersion.createdByName ?? '未知用户'} · {dateText(selectedVersion.createdAt)}；发布：{selectedVersion.publishedByName ?? '—'} · {dateText(selectedVersion.publishedAt)}</Typography.Paragraph>
          <Table rowKey="key" pagination={false} dataSource={selectedVersion.groups} scroll={{ x: 900 }} columns={[
            { title: '累计项', dataIndex: 'name' },
            { title: '成员', render: (_: unknown, group: DisciplineGroup) => group.terms.map((term) => term.inspectionTypeName).join('、') },
            { title: '单位', dataIndex: 'unit' },
            { title: '累计公式', render: (_: unknown, group: DisciplineGroup) => group.terms.map((term, index) => `${index ? (term.operator === '+' ? '+' : '−') + ' ' : ''}${term.inspectionTypeName} × ${term.factor} ${term.sourceUnit}`).join(' ') },
            { title: '处分档位', render: (_: unknown, group: DisciplineGroup) => group.tiers.map((tier) => `${tier.name} [${tier.lower}, ${tier.upper ?? '无上限'}) ${tier.clause} · ${tier.teacherConfirmationRequired ? '人工确认' : '免人工确认'}`).join('；') },
          ]} />
        </Card> : null
      })()}
    </section> : mode === 'view' ? <section className="discipline-config-view" aria-label="当前处分配置">
      {current.data?.current ? orderGroupsByInitialConfiguration(current.data.current.groups, versions.data?.items ?? []).map((group) => <Card key={group.key} className="discipline-view-group" title={<div className="discipline-view-group-heading">
          <div className="discipline-view-group-title"><span className="discipline-view-group-icon">{iconForGroup(group)}</span><Typography.Text strong>{group.name || '未命名累计项'}</Typography.Text><Tag color="blue">累计单位：{group.unit || '未配置'}</Tag><span className="discipline-view-tier-count">{group.tiers.length} 个处分档位</span></div>
          <div className="discipline-view-group-meta">
            <section aria-label={`${group.name}所含检查项`}><span>检查项</span><strong>{group.terms.map((term) => term.inspectionTypeName).join('、')}</strong></section>
            <section aria-label={`${group.name}换算方式`}><span>换算方式</span><div className="discipline-view-formula">{group.terms.map((term, index) => <span key={term.inspectionTypeId}>{index > 0 && (term.operator === '+' ? '＋ ' : '－ ')}<strong>{term.inspectionTypeName}</strong> × {term.factor} {term.sourceUnit} → {group.unit || '累计单位待配置'}</span>)}</div></section>
          </div>
        </div>}>
          <section aria-label={`${group.name}处分档位`}>
            <div className="discipline-view-tiers" tabIndex={0} role="region" aria-label={`${group.name}处分档位表格`}>
              <table><thead><tr><th scope="col">处分等级</th><th scope="col">处罚区间</th><th scope="col">确认方式</th></tr></thead>
                <tbody>{group.tiers.map((tier) => <tr key={tier.code}>
                  <th scope="row"><span>{tier.name || '未填写名称'}</span><small>依据：{tier.clause.trim() || '未填写依据'}</small></th>
                  <td>{`[${tier.lower}, ${tier.upper ?? '无上限'}) ${group.unit || ''}`}</td>
                  <td className={tier.teacherConfirmationRequired ? 'discipline-view-confirm-required' : undefined}>{tier.teacherConfirmationRequired ? '需要人工确认' : '免人工确认'}</td>
                </tr>)}</tbody>
              </table>
            </div>
          </section>
        </Card>) : <Card className="discipline-view-empty"><Empty description="当前还没有已发布的处分配置。" /><Typography.Paragraph type="secondary">创建配置草稿并完成检查后，即可发布为当前规则。</Typography.Paragraph></Card>}
    </section> : <>
      {version ? <>
        <Steps className="discipline-steps" current={step} items={stepTitles.map((title, index) => ({ title, status: stepHasValidationIssue(index) ? 'error' : index < step ? 'finish' : index === step ? 'process' : 'wait' }))} onChange={changeStep} />

        {step === 0 && <section className="discipline-step-workspace" aria-labelledby="step-one-heading">
          <div className="discipline-step-intro"><Typography.Title level={3} id="step-one-heading">把参与同一项累计的检查项放到一起</Typography.Title><Typography.Paragraph>每个检查项只能归属一个累计项。拖到“新建累计项”可新建一组，拖到已有组可合并；也可在累计项列表上方用选择操作完成。</Typography.Paragraph></div>
          <div className="discipline-grouping-controls">
            <div className="discipline-keyboard-move">
              <div className="discipline-keyboard-selects">
                <Select aria-label="选择检查项" placeholder="选择检查项" disabled={!editable} value={selectedTypeId} onChange={setSelectedTypeId} options={knownTypes.map((type) => ({ value: type.id, label: type.name }))} />
                <Select aria-label="选择目标累计项" placeholder="选择目标累计项" disabled={!editable} value={targetGroupKey} onChange={setTargetGroupKey} options={groups.map((group) => ({ value: group.key, label: group.name || '待命名累计项' }))} />
              </div>
              <div className="discipline-keyboard-actions">
                <Button disabled={!editable || !selectedTypeId || !targetGroupKey} onClick={() => selectedTypeId && targetGroupKey && moveType(selectedTypeId, targetGroupKey)}>移入所选累计项</Button>
                <Button disabled={!editable || !selectedTypeId} onClick={() => selectedTypeId && createGroupWithType(selectedTypeId)}>新建累计项并归入</Button>
              </div>
            </div>
            <div className={`discipline-new-group-target${dragOverKey === 'new-group' ? ' is-drop-target' : ''}`} onDragEnter={() => setDragOverKey('new-group')} onDragOver={(event) => { if (editable) event.preventDefault() }} onDrop={(event) => { event.preventDefault(); const id = event.dataTransfer.getData('text/plain'); setDragOverKey(undefined); if (editable && id) createGroupWithType(id) }}><DragOutlined aria-hidden="true" />拖到这里，新建累计项；拖入后以检查项名称自动命名</div>
          </div>
          <div className="discipline-grouping-layout">
            <Card title={`全部启用检查维度（${knownTypes.length}）`} className="discipline-source-list">
              {knownTypes.map((type) => { const assignedGroup = groupForType(type.id); return <div key={type.id} className="discipline-source-chip" draggable={editable && !assignedGroup} onDragStart={(event) => { if (!assignedGroup) event.dataTransfer.setData('text/plain', type.id) }}>
                {!assignedGroup && <DragOutlined className="discipline-drag-handle" aria-hidden="true" />}<strong>{type.name}</strong><Typography.Text type="secondary">{assignedGroup ? `已归入「${assignedGroup.name}」` : '未归组'}</Typography.Text>
              </div>})}
              <div className={`discipline-unassigned-drop${dragOverKey === 'unassigned' ? ' is-drop-target' : ''}`} onDragEnter={() => setDragOverKey('unassigned')} onDragOver={(event) => { if (editable) event.preventDefault() }} onDrop={(event) => { event.preventDefault(); const id = event.dataTransfer.getData('text/plain'); setDragOverKey(undefined); if (editable && id) moveToUnassigned(id) }}>将成员拖到这里可移回未归组</div>
            </Card>
            <Card title={`累计项（${groups.length}）`} extra={<Button disabled={!editable || undoGroups.length === 0} onClick={undoGroupChange}>撤销上一步</Button>} className="discipline-group-list">
              <div className="discipline-group-table-region" role="region" aria-label="累计项归组列表，可横向滚动" tabIndex={0}>
              <Table rowKey="key" dataSource={groups} pagination={false} columns={groupColumns} onRow={(group) => ({
                className: `discipline-drop-target${dragOverKey === group.key ? ' is-drop-target' : ''}`,
                onDragEnter: () => setDragOverKey(group.key),
                onDragOver: (event) => { if (editable) event.preventDefault() },
                onDragLeave: () => setDragOverKey(undefined),
                onDrop: (event) => { event.preventDefault(); setDragOverKey(undefined); const id = event.dataTransfer.getData('text/plain'); if (editable && id) moveType(id, group.key) },
              })} locale={{ emptyText: '拖入检查项到“新建累计项”区域，或使用列表上方的选择操作。' }} />
              </div>
            </Card>
          </div>
          <Typography.Text type="secondary">已归属 {exactlyAssignedCount} / {knownTypes.length} 个启用维度，每个维度只能归属一个累计项。</Typography.Text>
        </section>}

        {step === 1 && <section className="discipline-step-workspace" aria-labelledby="step-two-heading">
          <Typography.Title level={3} id="step-two-heading">确认计量单位与来源换算</Typography.Title>
          <div className="discipline-formula-layout">
            <nav className="discipline-group-nav" aria-label="选择累计项">{groups.map((group) => <Button key={group.key} block type={group.key === selectedGroup?.key ? 'primary' : 'default'} onClick={() => setSelectedGroupKey(group.key)}><span className="discipline-group-nav-label">{group.name}</span><span className="discipline-group-nav-meta">包含 {group.terms.length} 项检查 · {formulaNeedsReview.has(group.key) ? '需重审公式' : '公式已确认'}</span></Button>)}</nav>
            {selectedGroup && <Card title={`累计项：${selectedGroup.name}`} className="discipline-formula-editor">
              <Form layout="vertical"><Form.Item label="文书违纪事实名称"><Input disabled={!editable} maxLength={120} value={selectedGroup.violationName} onChange={event=>setGroupField(selectedGroup,'violationName',event.target.value)}/></Form.Item><Form.Item label="统一计量单位" {...fieldError(`groups[${groups.findIndex((group) => group.key === selectedGroup.key)}].unit`)}><Input id={fieldControlId(`groups[${groups.findIndex((group) => group.key === selectedGroup.key)}].unit`)} disabled={!editable} value={selectedGroup.unit} placeholder="例如：次、学时" onChange={(event) => setGroupField(selectedGroup, 'unit', event.target.value)} /></Form.Item></Form>
              {selectedGroup.terms.map((term, termIndex) => <div className="discipline-term-row" key={term.inspectionTypeId}>
                <Typography.Text strong>{termIndex ? (term.operator === '+' ? '加' : '减') : '首项'} · {term.inspectionTypeName}</Typography.Text>
                <Form layout="vertical" className="discipline-term-fields">
                  {termIndex > 0 && <Form.Item label="运算" {...fieldError(termPath(groups, selectedGroup, term, 'operator'))}><Select id={fieldControlId(termPath(groups, selectedGroup, term, 'operator'))} disabled={!editable} value={term.operator} options={[{ value: '+', label: '加' }, { value: '-', label: '减' }]} onChange={(operator) => setTermField(selectedGroup, term, { operator: operator as DisciplineTerm['operator'] }, 'operator')} /></Form.Item>}
                  <Form.Item label="来源单位" extra="按该检查项当前累计口径固定"><Input id={fieldControlId(termPath(groups, selectedGroup, term, 'sourceUnit'))} value={sourceUnitForType(term.inspectionTypeId)} disabled /></Form.Item>
                  <Form.Item label="换算系数（整数）" {...fieldError(termPath(groups, selectedGroup, term, 'factor'))}><InputNumber id={fieldControlId(termPath(groups, selectedGroup, term, 'factor'))} disabled={!editable} min={1} max={999999} precision={0} step={1} value={term.factor} onChange={(factor) => setTermField(selectedGroup, term, { factor: factor ?? 0 }, 'factor')} /></Form.Item>
                </Form>
                <Typography.Text className="discipline-formula-readable">{term.inspectionTypeName}原始值 × {term.factor} {termIndex ? (term.operator === '+' ? '+' : '−') : ''} → {selectedGroup.unit || '统一单位'}</Typography.Text>
              </div>)}
              {formulaNeedsReview.has(selectedGroup.key) && <Alert type="warning" showIcon title="检查项归属已变化，请核对本累计项所有换算系数和加减关系。" action={<Button size="small" onClick={() => confirmFormulaReview(selectedGroup.key)}>已核对公式</Button>} />}
              <Card size="small" title="公式示例" className="discipline-example-card">
                <Typography.Paragraph type="secondary">示例计算，不读取学生记录。</Typography.Paragraph>
                <Space wrap>{selectedGroup.terms.map((term) => <Form.Item key={term.inspectionTypeId} label={`${term.inspectionTypeName}示例值`}><InputNumber min={0} precision={0} step={1} aria-label={`${term.inspectionTypeName}示例值`} value={sampleValues[term.inspectionTypeId] ?? 0} onChange={(value) => setSampleValues((samples) => ({ ...samples, [term.inspectionTypeId]: value ?? 0 }))} /></Form.Item>)}</Space>
                <ul className="discipline-example-steps">{selectedGroup.terms.map((term, index) => <li key={term.inspectionTypeId}>{index ? (term.operator === '+' ? '加' : '减') + ' ' : ''}{term.inspectionTypeName}：{sampleValues[term.inspectionTypeId] ?? 0} × {term.factor} = {(sampleValues[term.inspectionTypeId] ?? 0) * term.factor} {selectedGroup.unit || ''}</li>)}</ul>
                {(() => { const result = calculateRuleExample(selectedGroup, sampleValues); return <Typography.Paragraph>累计结果：<strong>{result.total} {selectedGroup.unit || '单位待配置'}</strong>；档位：<strong>{result.tier?.name ?? '未命中已配置档位'}</strong></Typography.Paragraph> })()}
              </Card>
            </Card>}
          </div>
        </section>}

        {step === 2 && <section className="discipline-step-workspace" aria-labelledby="step-three-heading">
          <Typography.Title level={3} id="step-three-heading">按累计项配置处分档位</Typography.Title>
          <div className="discipline-tier-layout">
            <nav className="discipline-group-nav" aria-label="选择累计项配置档位">{groups.map((group) => <Button key={group.key} block type={group.key === selectedGroup?.key ? 'primary' : 'default'} onClick={() => setSelectedGroupKey(group.key)}><span className="discipline-group-nav-label">{group.name}</span><span className="discipline-group-nav-meta">累计单位：{group.unit || '待确认'}{tierNeedsReview.has(group.key) ? ' · 需重审档位' : ''}</span></Button>)}</nav>
            {selectedGroup && <Card title={<div className="discipline-tier-card-title"><span>{selectedGroup.name}</span><Tag color="blue">累计单位：{selectedGroup.unit || '待确认'}</Tag></div>} extra={<Tag className="discipline-tier-applicability">适用检查项：{selectedGroup.terms.map((term) => term.inspectionTypeName).join('、')}</Tag>} className="discipline-tier-editor">
              {tierNeedsReview.has(selectedGroup.key) && <Alert type="warning" showIcon title="归组或换算已变化，请重新核对本累计项的处分档位。" action={<Button size="small" onClick={() => confirmTierReview(selectedGroup.key)}>已核对档位</Button>} />}
              <Typography.Paragraph type="secondary">区间按“含起始、不含结束”计算。新增或移除档位后请核对阈值；继续前会检查区间是否连续且不重叠。</Typography.Paragraph>
              {levels.isLoading && <Typography.Text type="secondary">正在检查处分档位字典…</Typography.Text>}
              {levels.isError && <Alert type="error" showIcon title="处分档位字典类型加载失败" description="请检查管理端连接后重试；字典未加载时不能新增档位。" />}
              {!levels.isLoading && !levels.isError && !tierDictionary && <Alert type="error" showIcon title="处分档位字典未初始化" description="当前 API 返回中没有 DISCIPLINE_LEVEL 类型。请确认服务端已加载迁移 065，再刷新页面。" />}
              {!levels.isLoading && !levels.isError && tierOptions.length === 0 && <Alert type="warning" showIcon title="处分档位字典暂无启用项，暂时不能添加档位。" />}
              {issueAt(`groups[${groups.findIndex((group) => group.key === selectedGroup.key)}].tiers`) && <Typography.Text type="danger" role="alert">{issueAt(`groups[${groups.findIndex((group) => group.key === selectedGroup.key)}].tiers`)?.message}</Typography.Text>}
              <div className="discipline-tier-table-region" role="region" aria-label="处分档位表，可横向滚动" tabIndex={0}><Table rowKey="code" pagination={false} dataSource={selectedGroup.tiers} title={() => <div className="discipline-tier-toolbar"><div><Typography.Text strong>处分档位</Typography.Text><Typography.Text type="secondary">名称和依据默认只读，点击对应内容后编辑</Typography.Text></div><Button disabled={!editable || selectedGroup.tiers.length >= tierOptions.length || levels.isLoading || levels.isError || tierOptions.length === 0} onClick={() => addTier(selectedGroup)}>添加档位</Button></div>} columns={[
                { title: '处分名称', width: 220, render: (_: unknown, tier: DisciplineTier, index: number) => {
                  const path = `groups[${groups.findIndex((group) => group.key === selectedGroup.key)}].tiers[${index}].name`
                  const isEditing = tierTextEdit?.groupKey === selectedGroup.key && tierTextEdit.code === tier.code && tierTextEdit.field === 'name'
                  return <Form.Item {...fieldError(path)}><div className="discipline-tier-text-cell">{isEditing ? <><Input aria-label={`${tier.name}处分名称`} value={tierTextEdit.value} onChange={(event) => setTierTextEdit({ ...tierTextEdit, value: event.target.value })} /><Select aria-label="从处分档位字典填入名称" placeholder="从字典填入" value={undefined} options={tierOptions.map((option) => ({ value: option.label, label: option.label }))} onChange={(value: string) => setTierTextEdit((current) => current ? { ...current, value } : current)} /><Space size={4}><Button size="small" type="primary" onClick={() => saveTierText(selectedGroup, index)}>保存</Button><Button size="small" onClick={() => setTierTextEdit(undefined)}>取消</Button></Space></> : <Button className="discipline-tier-read-field" type="text" disabled={!editable} onClick={() => setTierTextEdit({ groupKey: selectedGroup.key, code: tier.code, field: 'name', value: tier.name })}>{tier.name || '点击填写处分名称'}</Button>}{issueAt(`groups[${groups.findIndex((group) => group.key === selectedGroup.key)}].tiers[${index}].code`) && <Typography.Text type="danger">{issueAt(`groups[${groups.findIndex((group) => group.key === selectedGroup.key)}].tiers[${index}].code`)?.message}</Typography.Text>}</div></Form.Item>
                } },
                { title: '起始值（含）', width: 120, render: (_: unknown, tier: DisciplineTier, index: number) => <Form.Item {...fieldError(`groups[${groups.findIndex((group) => group.key === selectedGroup.key)}].tiers[${index}].lower`)}><InputNumber id={fieldControlId(`groups[${groups.findIndex((group) => group.key === selectedGroup.key)}].tiers[${index}].lower`)} disabled={!editable} min={0} max={99999999} precision={0} step={1} value={tier.lower} onChange={(lower) => setTierField(selectedGroup, index, { lower: lower ?? 0 }, 'lower')} /></Form.Item> },
                { title: '结束值（不含）', width: 130, render: (_: unknown, tier: DisciplineTier, index: number) => <Form.Item {...fieldError(`groups[${groups.findIndex((group) => group.key === selectedGroup.key)}].tiers[${index}].upper`)}><InputNumber id={fieldControlId(`groups[${groups.findIndex((group) => group.key === selectedGroup.key)}].tiers[${index}].upper`)} disabled={!editable || index === selectedGroup.tiers.length - 1} min={0} max={99999999} precision={0} step={1} placeholder={index === selectedGroup.tiers.length - 1 ? '无上限' : undefined} value={tier.upper} onChange={(upper) => setTierField(selectedGroup, index, { upper: upper ?? undefined }, 'upper')} /></Form.Item> },
                { title: '依据条款', width: 230, render: (_: unknown, tier: DisciplineTier, index: number) => {
                  const path = `groups[${groups.findIndex((group) => group.key === selectedGroup.key)}].tiers[${index}].clause`
                  const isEditing = tierTextEdit?.groupKey === selectedGroup.key && tierTextEdit.code === tier.code && tierTextEdit.field === 'clause'
                  return <Form.Item {...fieldError(path)}><div className="discipline-tier-text-cell">{isEditing ? <><Input.TextArea id={fieldControlId(path)} aria-label={`${tier.name}依据条款`} autoSize={{ minRows: 2, maxRows: 4 }} value={tierTextEdit.value} onChange={(event) => setTierTextEdit({ ...tierTextEdit, value: event.target.value })} /><Space size={4}><Button size="small" type="primary" onClick={() => saveTierText(selectedGroup, index)}>保存</Button><Button size="small" onClick={() => setTierTextEdit(undefined)}>取消</Button></Space></> : <Button className="discipline-tier-read-field discipline-tier-clause-field" type="text" disabled={!editable} onClick={() => setTierTextEdit({ groupKey: selectedGroup.key, code: tier.code, field: 'clause', value: tier.clause })}>{tier.clause || '点击填写处分依据'}</Button>}</div></Form.Item>
                } },
                { title:'文书依据',width:208,render:(_:unknown,tier:DisciplineTier,index:number)=><Space orientation="vertical" size={4}>
                  <Input disabled={!editable} aria-label={tier.name+'依据文件文号'} placeholder="依据文件文号" maxLength={120} value={tier.authorityNumber} onChange={event=>setTierField(selectedGroup,index,{authorityNumber:event.target.value},'authorityNumber')}/>
                  <Input disabled={!editable} aria-label={tier.name+'章条款定位'} placeholder="章条款定位" maxLength={120} value={tier.clauseReference} onChange={event=>setTierField(selectedGroup,index,{clauseReference:event.target.value},'clauseReference')}/>
                </Space> },
                { title: '需要人工确认', width: 145, render: (_: unknown, tier: DisciplineTier, index: number) => <div className="discipline-confirm-switch"><Switch checkedChildren="是" unCheckedChildren="否" aria-label={`${tier.name}需要人工确认`} disabled={!editable} checked={tier.teacherConfirmationRequired} onChange={(teacherConfirmationRequired) => setTierField(selectedGroup, index, { teacherConfirmationRequired }, 'teacherConfirmationRequired')} /></div> },
                { title: '操作', width: 82, render: (_: unknown, _tier: DisciplineTier, index: number) => <Button danger type="link" disabled={!editable || selectedGroup.tiers.length === 1} onClick={() => removeTier(selectedGroup, index)}>移除</Button> },
              ]} /></div>
            </Card>}
          </div>
        </section>}

        {step === 3 && <section className="discipline-step-workspace" aria-labelledby="step-four-heading">
          <Typography.Title level={3} id="step-four-heading">检查规则并发布</Typography.Title>
          {groups.map((group) => <Card key={group.key} className="discipline-rule-summary" style={{ marginTop: 10 }} title={<Space wrap><Typography.Text strong>{group.name}</Typography.Text><Tag color="blue">{group.unit || '单位待配置'}</Tag></Space>}>
            <section className="discipline-summary-section" aria-label={`${group.name}检查项`}>
              <Typography.Text className="discipline-summary-label">检查项</Typography.Text>
              <Space wrap size={[6, 6]}>{group.terms.map((term) => <Tag key={term.inspectionTypeId}>{term.inspectionTypeName}</Tag>)}</Space>
            </section>
            <section className="discipline-summary-section" aria-label={`${group.name}换算公式`}>
              <Typography.Text className="discipline-summary-label">换算公式</Typography.Text>
              <div className="discipline-summary-terms">{group.terms.map((term, index) => <div key={term.inspectionTypeId} className="discipline-summary-term">
                <Typography.Text strong>{term.inspectionTypeName}</Typography.Text>
                <Typography.Text type="secondary">{sourceUnitForType(term.inspectionTypeId)}</Typography.Text>
                <Typography.Text>{index === 0 ? '首项' : term.operator === '+' ? '加' : '减'}</Typography.Text>
                <Typography.Text>× {term.factor}</Typography.Text>
                <Typography.Text type="secondary">→ {group.unit || '累计单位待配置'}</Typography.Text>
              </div>)}</div>
            </section>
            <section className="discipline-summary-section" aria-label={`${group.name}处分档位`}>
              <Typography.Text className="discipline-summary-label">处分档位</Typography.Text>
              <div className="discipline-summary-tiers">{group.tiers.map((tier) => <div key={tier.code} className="discipline-summary-tier">
                <div><Typography.Text className="discipline-summary-field-label">名称</Typography.Text><Typography.Text strong>{tier.name || '未填写名称'}</Typography.Text></div>
                <div><Typography.Text className="discipline-summary-field-label">累计区间</Typography.Text><Typography.Text>[{tier.lower}, {tier.upper ?? '无上限'}) {group.unit}</Typography.Text></div>
                <div><Typography.Text className="discipline-summary-field-label">依据</Typography.Text><Typography.Text>{tier.clause.trim() || '未填写依据'}</Typography.Text></div>
                <div><Typography.Text className="discipline-summary-field-label">确认方式</Typography.Text><Tag color={tier.teacherConfirmationRequired ? 'gold' : 'default'}>{tier.teacherConfirmationRequired ? '需要人工确认' : '免人工确认'}</Tag></div>
              </div>)}</div>
            </section>
          </Card>)}
        </section>}

        {editable && <footer className="discipline-step-footer">
          <div aria-live="polite" className="discipline-save-status">{dirty ? '有未保存更改' : lastSavedAt ? `已保存 · ${dateText(lastSavedAt)}` : version.status === 'DRAFT' ? '草稿尚未修改' : '只读版本'}{saveDraft.isPending ? ' · 正在保存…' : ''}{validating ? ' · 正在校验…' : ''}{publishRule.isPending ? ' · 正在发布…' : ''}</div>
          <Space wrap>
            <Button disabled={step === 0 || !editable} onClick={() => changeStep(step - 1)}>上一步</Button>
            {step < 3
              ? <Button type="primary" disabled={!editable} onClick={() => changeStep(step + 1)}>继续：{stepTitles[step + 1]}</Button>
              : <Button type="primary" disabled={!canPublish || !editable || publishRule.isPending || validating} loading={publishRule.isPending || validating} onClick={() => void validateAndPreparePublish()}>校验并发布</Button>}
            <Button disabled={!editable || !dirty} loading={saveDraft.isPending} onClick={() => version && saveDraft.mutate(version)}>保存草稿</Button>
          </Space>
        </footer>}
      </> : <Card title="处分规则配置"><Empty description={noDraft ? '新建草稿后，从当前启用的日常检查维度开始归组。' : '请选择规则版本或返回配置。'} /></Card>}
    </>}
    <Modal title="有未保存的更改" open={!!pendingTransition} onCancel={() => setPendingTransition(undefined)} footer={[
      <Button key="stay" onClick={() => setPendingTransition(undefined)}>继续编辑</Button>,
      <Button key="discard" danger onClick={discardAndTransition}>放弃更改并继续</Button>,
      <Button key="save" type="primary" loading={saveDraft.isPending} onClick={() => void saveAndTransition()}>保存并继续</Button>,
    ]}>
      <Typography.Paragraph>离开规则配置、切换历史版本或前往其他页面前，请选择如何处理未保存的更改。</Typography.Paragraph>
    </Modal>
    <Modal title="确认发布默认规则" open={publishConfirmOpen} onCancel={() => setPublishConfirmOpen(false)} onOk={() => publishTarget && publishRule.mutate({ id: publishTarget.id, rowVersion: publishTarget.rowVersion })} confirmLoading={publishRule.isPending} okText="确认发布" cancelText="返回检查">
      <Typography.Paragraph>规则版本 v{publishTarget?.version} 将成为全院默认配置。发布只切换规则版本；这里不包含学生影响数量或待处分重算结果。</Typography.Paragraph>
    </Modal>
    <Modal title="设置新增档位起始值" open={Boolean(tierBoundaryPrompt)} onCancel={() => { setTierBoundaryPrompt(undefined); setTierBoundaryValue(undefined) }} onOk={confirmTierBoundary} okText="确认并添加" cancelText="取消">
      <Typography.Paragraph>该档位将排在最高档。请填写新档起始值；确认后此值会同时成为前一档结束值，系统不会代填处分阈值。</Typography.Paragraph>
      <Form.Item label="新档起始值" required>
        <InputNumber aria-label="新档起始值" min={(tierBoundaryPrompt?.minimum ?? 0) + 1} max={99999999} precision={0} step={1} value={tierBoundaryValue} onChange={(value) => setTierBoundaryValue(value ?? undefined)} />
      </Form.Item>
      {tierBoundaryPrompt && <Typography.Text type="secondary">必须大于当前最高档起始值 {tierBoundaryPrompt.minimum}</Typography.Text>}
    </Modal>
  </section>
}

function termPath(groups: DisciplineGroup[], group: DisciplineGroup, term: DisciplineTerm, field: string) {
  return `groups[${groups.findIndex((item) => item.key === group.key)}].terms[${group.terms.findIndex((item) => item.inspectionTypeId === term.inspectionTypeId)}].${field}`
}
