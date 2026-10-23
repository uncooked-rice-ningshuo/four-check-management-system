import { Alert, Button, Drawer, Empty, Modal, Select, Space, Table, Tag, Typography, message } from 'antd'
import { useState } from 'react'
import { type HygieneArea, type HygieneAreaTarget } from '../../shared/api/hygieneAreaApi'

import { useHygieneAreas, useHygieneAreaTargets, useHygieneAreaReferences, useHygieneAreaChanges, useHygieneAreaSave } from '../../features/hygiene-area/model/queries'
import { HygieneAreaEditor } from '../../features/hygiene-area/ui/HygieneAreaEditor'

const targetKey = (target: HygieneAreaTarget) => `${target.kind}:${target.departmentId}`
const areaLabel = (area: HygieneArea, parents: { id: string; name: string }[]) => {
  const parent = parents.find(item => item.id === (area.classroomId || area.dormitoryBuildingId))
  return [parent?.name, area.floorUnit, area.name].filter(Boolean).join(' / ')
}

/** Scoped configuration for the two hygiene area catalogs. Every mutation
 * carries a department target and the server rechecks the live grant. */
export function HygieneAreaPage() {
  const [selection, setSelection] = useState<string>()
  const [page, setPage] = useState(1)
  const [editing, setEditing] = useState<HygieneArea | 'new'>()
  const [history, setHistory] = useState<HygieneArea>()
  const targets = useHygieneAreaTargets()
  const target = targets.data?.find(item => targetKey(item) === selection) ?? targets.data?.[0]
  const areas = useHygieneAreas(target, page)
  const references = useHygieneAreaReferences(target)
  const changes = useHygieneAreaChanges(target, history?.id)
  const toggle = useHygieneAreaSave()
  const [messageApi, messageContext] = message.useMessage()
  const [modalApi, modalContext] = Modal.useModal()
  const openEditor = (area: HygieneArea | 'new') => setEditing(area)
  const errors = [targets, areas, references].filter(query => query.isError)
  return <section style={{ minWidth: 0 }}>
    {messageContext}{modalContext}
    <Typography.Title level={2}>卫生区域维护</Typography.Title>
    <Typography.Paragraph type="secondary">按部门和检查类型的授权范围维护区域；停用后不再用于新记录。</Typography.Paragraph>
    {targets.isLoading ? <Typography.Text>正在加载维护范围…</Typography.Text> : null}
    {errors.length > 0 && <Alert type="error" title="区域配置加载失败，请重试" action={<Button onClick={() => errors.forEach(query => void query.refetch())}>重试</Button>} />}
    {targets.data?.length === 0 && <Empty description="当前账号没有卫生区域维护权限" />}
    {target && <>
      <Space wrap style={{ marginBottom: 16 }}>
        <Select aria-label="维护范围" style={{ minWidth: 280 }} value={targetKey(target)} options={targets.data?.map(item => ({ value: targetKey(item), label: `${item.inspectionTypeName} · ${item.departmentName}` }))} onChange={value => { setSelection(value); setPage(1); setEditing(undefined); setHistory(undefined) }} />
        <Button type="primary" onClick={() => openEditor('new')} disabled={references.isError || references.isLoading}>新增区域</Button>
      </Space>
      <Table<HygieneArea> rowKey="id" dataSource={areas.isError ? [] : areas.data?.items ?? []} loading={areas.isLoading} scroll={{ x: 720 }} pagination={{ current: page, pageSize: 20, total: areas.data?.total ?? 0, onChange: setPage }} locale={{ emptyText: areas.isError ? '区域加载失败，请重试' : '暂无区域' }} columns={[
        { title: '区域', key: 'name', render: (_, area) => areaLabel(area, references.data ?? []) || area.name },
        { title: '状态', dataIndex: 'enabled', width: 90, render: enabled => <Tag color={enabled ? 'green' : 'default'}>{enabled ? '启用' : '停用'}</Tag> },
        { title: '操作', key: 'actions', width: 240, render: (_, area) => <Space><Button type="link" disabled={references.isError || references.isLoading} onClick={() => openEditor(area)}>编辑</Button><Button type="link" onClick={() => setHistory(area)}>变更记录</Button><Button type="link" danger={area.enabled} loading={toggle.isPending} onClick={() => modalApi.confirm({ title: area.enabled ? '停用此区域？' : '启用此区域？', content: areaLabel(area, references.data ?? []), onOk: async () => { if (!target) return; await toggle.mutateAsync({ target, value: { ...area, enabled: !area.enabled }, id: area.id }); void messageApi.success('区域状态已更新') } })}>{area.enabled ? '停用' : '启用'}</Button></Space> },
      ]} />
    </>}
    {editing && target && <HygieneAreaEditor key={targetKey(target) + ':' + (editing === 'new' ? 'new' : editing.id)} target={target} area={editing} parents={references.data ?? []} onClose={() => setEditing(undefined)} />}
    <Drawer open={!!history} title="区域变更记录" onClose={() => setHistory(undefined)} destroyOnHidden>
      {changes.isError ? <Alert type="error" title="变更记录加载失败" action={<Button onClick={() => void changes.refetch()}>重试</Button>} /> : <Table rowKey="id" loading={changes.isLoading} dataSource={changes.data ?? []} pagination={false} columns={[{ title: '操作', dataIndex: 'action' }, { title: '时间', dataIndex: 'createdAt' }, { title: '操作人', dataIndex: 'actorId' }]} />}
    </Drawer>
  </section>
}
