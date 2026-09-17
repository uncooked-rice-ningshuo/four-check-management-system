import { Alert, Drawer, Form, Input, Modal, Skeleton } from 'antd'
import { useRef, useState } from 'react'
import type { AttendanceFilterOptions, AttendanceRecord } from '../../../entities/inspection/model/inspection'
import { useAttendanceDetail, useDeleteAttendance } from '../hooks/useAttendanceMutations'
import { ManualAttendanceDrawer } from './ManualAttendanceDrawer'
import { ApiError } from '../../../shared/api/httpClient'

type Props = { options?: AttendanceFilterOptions; editId?: string; deleting: AttendanceRecord[]; onClose: () => void }
export function AttendanceMutationDialogs({ options, editId, deleting, onClose }: Props) {
  const detail = useAttendanceDetail(editId)
  const remove = useDeleteAttendance()
  const [form] = Form.useForm<{ reason: string }>()
  const [error, setError] = useState('')
  const busy = useRef(false)
  const confirm = async () => {
    if (busy.current) return
    let values: { reason: string }
    try { values = await form.validateFields() } catch { return }
    busy.current = true
    setError('')
    try {
      await remove.mutateAsync({ items: deleting.map(record => ({ id: record.id, version: record.rowVersion })), reason: values.reason })
      onClose()
    } catch (failure) {
      setError(failure instanceof ApiError ? failure.message : '删除失败，请稍后重试')
    } finally { busy.current = false }
  }
  return <>
    {editId && (detail.data && options ? <ManualAttendanceDrawer editing={detail.data} options={options} onClose={onClose} /> : <Drawer open title="编辑到课率记录" onClose={onClose}>{detail.isError ? <Alert type="error" showIcon title="记录不存在或无权操作，请刷新列表" /> : <Skeleton active />}</Drawer>)}
    {deleting.length > 0 && <Modal open title={`确认删除 ${deleting.length} 条到课率草稿？`} okText="确认删除" cancelText="取消" okButtonProps={{ danger: true }} confirmLoading={remove.isPending} closable={!remove.isPending} maskClosable={!remove.isPending} cancelButtonProps={{ disabled: remove.isPending }} onCancel={() => { if (!busy.current) onClose() }} onOk={() => void confirm()}>
      <p>将软删除所选草稿并保留审计记录。任一记录已提交、锁定、越权或版本变化时，整批不会删除。</p>
      {error && <Alert type="error" showIcon title={error} />}
      <Form form={form} layout="vertical" disabled={remove.isPending}><Form.Item name="reason" label="删除原因" rules={[{ required: true, whitespace: true, message: '请填写删除原因' }]}><Input.TextArea maxLength={255} rows={3} /></Form.Item></Form>
    </Modal>}
  </>
}
