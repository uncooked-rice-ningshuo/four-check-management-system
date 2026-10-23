import { Alert, Button, Drawer, Form, Input, Select, message } from 'antd'
import type { HygieneArea, HygieneAreaTarget, HygieneParent } from '../../../shared/api/hygieneAreaApi'
import { useHygieneAreaSave } from '../model/queries'

type Values = Omit<HygieneArea, 'id' | 'kind' | 'enabled'>
/** Each mounted editor owns its form and request state, avoiding values leaking between records. */
export function HygieneAreaEditor({ target, area, parents, onClose }: { target: HygieneAreaTarget; area: HygieneArea | 'new'; parents: HygieneParent[]; onClose: () => void }) {
  const save = useHygieneAreaSave()
  const [messageApi, messageContext] = message.useMessage()
  const isNew = area === 'new'
  return <Drawer open title={isNew ? '新增卫生区域' : '编辑卫生区域'} onClose={save.isPending ? undefined : onClose} closable={!save.isPending} maskClosable={!save.isPending} destroyOnHidden size={520}>
    {messageContext}
    <Form<Values> layout="vertical" initialValues={isNew ? { name: '', floorUnit: '' } : area} onFinish={value => save.mutate({ target, value: { ...value, enabled: isNew ? true : area.enabled }, id: isNew ? undefined : area.id }, { onSuccess: () => { void messageApi.success('区域已保存'); onClose() } })}>
      <Form.Item label={target.kind === 'classroom-hygiene-areas' ? '教室' : '宿舍楼'} name={target.kind === 'classroom-hygiene-areas' ? 'classroomId' : 'dormitoryBuildingId'} rules={[{ required: true, message: '请选择关联对象' }]}><Select showSearch optionFilterProp="label" options={parents.map(item => ({ value: item.id, label: item.name }))} /></Form.Item>
      {target.kind === 'public-dormitory-hygiene-areas' && <Form.Item label="楼层/单元" name="floorUnit" rules={[{ required: true, whitespace: true, message: '请输入楼层或单元' }]}><Input maxLength={64} /></Form.Item>}
      <Form.Item label="区域名称" name="name" rules={[{ required: true, whitespace: true, message: '请输入区域名称' }]}><Input maxLength={128} /></Form.Item>
      {save.isError && <Alert type="error" title="保存失败，请检查输入或权限后重试" style={{ marginBottom: 16 }} />}
      <Button type="primary" htmlType="submit" loading={save.isPending}>保存</Button>
    </Form>
  </Drawer>
}
