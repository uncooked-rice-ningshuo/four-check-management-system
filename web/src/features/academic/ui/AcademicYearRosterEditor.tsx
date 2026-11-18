import { useQuery } from '@tanstack/react-query'
import { useState, type ReactNode } from 'react'
import { ArrowUpOutlined, ArrowDownOutlined, HolderOutlined, ReadOutlined, ApartmentOutlined, CaretDownFilled } from '@ant-design/icons'
import { naturalClassOrder } from '../model/classOrder'
import './academic-roster.css'
import { Alert, Button, Select, Popconfirm, Typography, Tree, Steps, ConfigProvider } from 'antd'
import type { AcademicYear, YearRoster } from '../../../entities/academic/model/academic'
import type { MasterDataRecord } from '../../../entities/masterdata/model/masterData'
import { getMasterData } from '../../../shared/api/adminApi'

async function loadOptions(kind: 'grades' | 'majors' | 'classes') {
  const items: MasterDataRecord[] = []
  for (let page = 1; ; page++) {
    const result = await getMasterData(kind, { page, pageSize: 100 })
    items.push(...result.items)
    if (items.length >= result.total) return items
    if (!result.items.length) throw new Error('名单选项加载不完整')
  }
}

function useRosterCatalog() {
  return useQuery({ queryKey: ['academic', 'roster-options'], queryFn: () => Promise.all([loadOptions('grades'), loadOptions('majors'), loadOptions('classes')]) })
}

type SortListProps = {
  ids: string[]
  catalog: MasterDataRecord[]
  editable: boolean
  onSort: (ids: string[]) => void
  actionOf?: (id: string) => ReactNode
  labelOf?: (id: string, name: string) => ReactNode
  childrenOf?: (id: string) => ReactNode
}

function SortList({ ids, catalog, editable, onSort, actionOf, labelOf, childrenOf }: SortListProps) {
  const [dragged, setDragged] = useState<string>()
  function move(from: number, to: number) {
    if (from < 0 || from === to) return
    const next = [...ids]
    next.splice(to, 0, ...next.splice(from, 1))
    onSort(next)
  }
  return <ol className="roster-sort-list">{ids.map((id, index) => {
    const name = catalog.find(item => item.id === id)?.name ?? '已停用项目'
    return <li key={id}><div className="roster-sort-row"
      onDragOver={event => { if (editable && dragged) event.preventDefault() }}
      onDrop={event => { if (editable && dragged) { event.preventDefault(); move(ids.indexOf(dragged), index); setDragged(undefined) } }}>
      {editable && <span className="roster-grip" draggable onDragStart={() => setDragged(id)} onDragEnd={() => setDragged(undefined)}><HolderOutlined /></span>}
      {labelOf ? labelOf(id, name) : <strong>{name}</strong>}{actionOf?.(id)}
      {editable && <span className="roster-moves">{[-1, 1].map(direction => <Button key={direction} type="text" size="small"
        aria-label={(direction < 0 ? '上移' : '下移') + name} disabled={!ids[index + direction]}
        icon={direction < 0 ? <ArrowUpOutlined /> : <ArrowDownOutlined />} onClick={() => move(index, index + direction)} />)}</span>}
    </div>{childrenOf?.(id)}</li>
  })}</ol>
}

type GradeRoster = YearRoster['grades'][number]

type MembershipNode = {
  key: string
  title: string
  gradeId: string
  majorId?: string
  classId?: string
  classCount?: number
  children?: MembershipNode[]
}

function MembershipTree({ grade, majors, classes, onChange }: {
  grade: GradeRoster
  majors: MasterDataRecord[]
  classes: MasterDataRecord[]
  onChange: (majors: GradeRoster['majors']) => void
}) {
  const nodes: MembershipNode[] = majors.filter(item => item.enabled || grade.majors.some(m => m.majorId === item.id)).map(item => {
      const major = grade.majors.find(m => m.majorId === item.id)
      const options = classes.filter(c => c.gradeId === grade.gradeId && c.majorId === item.id && (c.enabled || major?.classIds.includes(c.id)))
      return {
        classCount: options.filter(c => c.enabled).length, key: grade.gradeId + '/' + item.id, title: item.name, gradeId: grade.gradeId, majorId: item.id,
        children: naturalClassOrder(options.map(c => c.id), options).map(id => ({
          key: 'class/' + id, title: options.find(c => c.id === id)?.name ?? '已停用班级',
          gradeId: grade.gradeId, majorId: item.id, classId: id,
        })),
      }
    })
  const checked = grade.majors.flatMap(m => [grade.gradeId + '/' + m.majorId, ...m.classIds.map(id => 'class/' + id)])
  return <ConfigProvider theme={{ components: { Tree: { titleHeight: 36 } } }}><Tree<MembershipNode> className="roster-membership-tree" aria-label="专业与班级" switcherIcon={<CaretDownFilled aria-hidden />} showLine={{ showLeafIcon: false }} blockNode checkable checkStrictly expandAction="click" selectedKeys={[]}
    treeData={nodes} checkedKeys={checked}
    defaultExpandedKeys={grade.majors.filter(m => m.classIds.length > 0).map(m => grade.gradeId + '/' + m.majorId)}
    titleRender={node => node.classId ? node.title : <div className="roster-major-label"><span>{node.title}</span><small>{node.classCount} 个班</small></div>}
    onCheck={(_, { node, checked: selected }) => {
      const majorId = node.majorId
      if (!majorId) return
      const major = grade.majors.find(m => m.majorId === majorId)
      let next: GradeRoster['majors']
      if (node.classId) {
        const prior = major?.classIds ?? []
        const ids = selected ? [...prior, node.classId] : prior.filter(id => id !== node.classId)
        const automatic = naturalClassOrder(prior, classes).every((id, i) => id === prior[i])
        const updated = { majorId, classIds: automatic ? naturalClassOrder(ids, classes) : ids }
        next = major ? grade.majors.map(m => m === major ? updated : m) : [...grade.majors, updated]
      } else {
        const ids = (node.children ?? []).flatMap(c => c.classId ? [c.classId] : [])
        next = selected ? [...grade.majors, { majorId, classIds: ids }] : grade.majors.filter(m => m !== major)
      }
      onChange(next)
    }} /></ConfigProvider>
}

/** Displays the saved export order without mounting the configuration form. */
export function AcademicYearOrderView({ roster }: { roster?: YearRoster }) {
  const catalog = useRosterCatalog()
  if (!roster?.grades.length) return <Typography.Paragraph>该学年尚未配置名单。</Typography.Paragraph>
  if (catalog.isError) return <Alert type="error" title="名单名称加载失败" action={<Button onClick={() => void catalog.refetch()}>重试</Button>} />
  if (!catalog.data) return <Typography.Text>正在加载名单…</Typography.Text>
  const names = catalog.data.map(items => new Map(items.map(item => [item.id, item.name])))
  return <section className="roster-preview roster-readonly" aria-label="已保存的展示顺序">
    <ol className="roster-sort-list">{roster.grades.map(grade => <li key={grade.gradeId}>
      <details open><summary><strong>{names[0].get(grade.gradeId) ?? '已停用年级'}</strong><small>{grade.majors.length} 个专业</small></summary>
        <ol className="roster-sort-list roster-view-majors">{grade.majors.map(major => {
          const label = <><strong>{names[1].get(major.majorId) ?? '已停用专业'}</strong><small>{major.classIds.length} 个班级</small></>
          return <li key={major.majorId}>{major.classIds.length ? <details><summary>{label}</summary>
            <ol className="roster-sort-list roster-classes">{major.classIds.map(id =>
              <li key={id} className="roster-sort-row">{names[2].get(id) ?? '已停用班级'}</li>)}</ol>
          </details> : <div className="roster-view-leaf">{label}</div>}</li>
        })}</ol>
      </details>
    </li>)}</ol>
  </section>
}

export function AcademicYearRosterEditor({ value, onChange, years, startYear, step }: { step: number; startYear: number; years: AcademicYear[]; value?: YearRoster; onChange?: (value: YearRoster) => void }) {
  const [selectedGrade, setSelectedGrade] = useState<string>()
  const [expandedMajors, setExpandedMajors] = useState<string[]>([])
  const [sorting, setSorting] = useState('')
  const previous = years.find(year => year.roster && Number(year.startDate.slice(0, 4)) < startYear)
  const roster = value ?? { grades: [], majorIds: [] }
  const catalog = useRosterCatalog()
  if (catalog.isError) return <Alert type="error" title="名单选项加载失败" action={<Button onClick={() => void catalog.refetch()}>重试</Button>} />
  if (!catalog.data) return <Typography.Text>正在加载名单选项…</Typography.Text>
  const [grades, majors, classes] = catalog.data
  const previousRoster = previous?.roster
  const saveGrades = (next: YearRoster['grades']) => onChange?.({ grades: next, majorIds: [...new Set(next.flatMap(g => g.majors.map(m => m.majorId)))] })
  const active = roster.grades.find(g => g.gradeId === selectedGrade) ?? roster.grades[0]
  const gradeName = (id: string) => grades.find(g => g.id === id)?.name ?? '已停用年级'
  const updateMajors = (gradeId: string, next: YearRoster['grades'][number]['majors']) =>
    saveGrades(roster.grades.map(g => g.gradeId === gradeId ? { ...g, majors: next } : g))
  const sortToggle = (key: string, label: string) => <Button size="small" type="link" aria-pressed={sorting === key}
    onClick={() => setSorting(sorting === key ? '' : key)}>{sorting === key ? '完成调序' : label}</Button>
  return <div className="roster-editor">
    <Steps size="small" current={step} items={[{ title: '配置年级与专业' }, { title: '调整展示顺序' }]} />
    {step === 0 ? <div className="roster-workspace">
      <aside className="roster-sidebar"><h3>本学年年级</h3>
        <Select showSearch optionFilterProp="label" aria-label="学年添加年级" value={null} placeholder="添加年级"
          options={grades.filter(item => item.enabled && !roster.grades.some(g => g.gradeId === item.id)).map(item => ({ label: item.name, value: item.id }))}
          onChange={(gradeId: string) => { saveGrades([...roster.grades, { gradeId, majors: [] }]); setSelectedGrade(gradeId) }} />
      {roster.grades.map(g => <button key={g.gradeId} type="button" className="roster-grade-choice" aria-pressed={active === g} onClick={() => setSelectedGrade(g.gradeId)}><span>{gradeName(g.gradeId)}</span><small>{g.majors.length} 个专业</small></button>)}
      {!value && previousRoster && <Button type="link" onClick={() => onChange?.(structuredClone(previousRoster))}>复制{previous?.name}名单</Button>}
      </aside>
      {active && <section className="roster-selection"><header><strong>{gradeName(active.gradeId)} · 开设专业</strong>
        <Popconfirm title={`移除${gradeName(active.gradeId)}及下属名单？`} okText="移除" cancelText="取消"
          onConfirm={() => saveGrades(roster.grades.filter(g => g !== active))}><Button type="text" size="small">移除年级</Button></Popconfirm></header>
        <div className="roster-column-heading">本年级班级数</div>
        <MembershipTree key={active.gradeId} grade={active} majors={majors} classes={classes} onChange={next => updateMajors(active.gradeId, next)} /></section>}
      {!roster.grades.length && <div className="roster-empty"><ReadOutlined /><h3>先添加一个年级</h3><p>然后为它选择本学年开设的专业。</p></div>}
    </div> : <section className="roster-preview"><div className="roster-heading"><div><h3><ApartmentOutlined /> 导出展示顺序</h3><p>Word 和 Excel 将按下方顺序展示。</p></div>
      {roster.grades.length > 1 && sortToggle('grades', '调整年级顺序')}</div>
      <SortList ids={roster.grades.map(g => g.gradeId)} catalog={grades} editable={sorting === 'grades'}
        onSort={ids => saveGrades(ids.flatMap(id => roster.grades.filter(g => g.gradeId === id)))}
        actionOf={id => {
          const count = roster.grades.find(g => g.gradeId === id)?.majors.length ?? 0
          return <><small>{count} 个专业</small>{count > 1 && sortToggle(id, '调整专业顺序')}</>
        }} childrenOf={gradeId => {
          const grade = roster.grades.find(g => g.gradeId === gradeId)
          if (!grade) return null
          return <div className="roster-major-list">
            <SortList ids={grade.majors.map(m => m.majorId)} catalog={majors} editable={sorting === gradeId}
              onSort={ids => updateMajors(gradeId, ids.flatMap(id => grade.majors.filter(m => m.majorId === id)))}
              labelOf={(majorId, name) => {
                const count = grade.majors.find(m => m.majorId === majorId)?.classIds.length ?? 0
                const key = gradeId + '/' + majorId
                const expanded = expandedMajors.includes(key)
                return count ? <button className="roster-disclosure" type="button" aria-expanded={expanded} onClick={() =>
                  setExpandedMajors(current => expanded ? current.filter(id => id !== key) : [...current, key])}><strong>{name}</strong><span>{count} 个班级 {expanded ? '▴' : '▾'}</span></button> : <><strong>{name}</strong><small>暂无班级</small></>
              }} childrenOf={majorId => {
                const major = grade.majors.find(m => m.majorId === majorId)
                if (!major?.classIds.length) return null
                const key = gradeId + '/' + majorId
                const setClasses = (classIds: string[]) => updateMajors(gradeId, grade.majors.map(m => m === major ? { ...m, classIds } : m))
                if (!expandedMajors.includes(key)) return null
                return <div className="roster-classes">
                  {major.classIds.length > 1 && sortToggle(key, '自定义班级顺序')}{sorting === key && <Button type="link" size="small" onClick={() => setClasses(naturalClassOrder(major.classIds, classes))}>恢复班号升序</Button>}
                  <SortList ids={major.classIds} catalog={classes} editable={sorting === key} onSort={setClasses} /></div>
              }} /></div>
        }} /></section>}
  </div>
}
