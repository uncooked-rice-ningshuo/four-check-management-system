import { Alert, Button, Input, Modal, Space } from 'antd'
import { useState } from 'react'
import type { EveningSelfStudyRecord } from '../../../entities/inspection/model/eveningSelfStudyRecord'
import { useEveningSelfStudyDelete } from '../model/queries'

export function EveningSelfStudyDeleteDialog({ records, onClose }: { records: EveningSelfStudyRecord[]; onClose: () => void }) {
 const [reason, setReason] = useState('')
 const [error, setError] = useState('')
 const deletion = useEveningSelfStudyDelete()
 const submit = async () => {
  if (!reason.trim()) { setError('请填写删除原因。'); return }
  setError('')
  try {
   await deletion.mutateAsync({ items: records.map(record => ({ id: record.id, version: record.rowVersion })), reason: reason.trim() })
   onClose()
  } catch { setError('删除失败，记录可能已变化或权限已调整；请刷新列表后重试。') }
 }
 return <Modal open title={`删除 ${records.length} 条晚自习草稿`} onCancel={onClose} footer={<Space><Button disabled={deletion.isPending} onClick={onClose}>取消</Button><Button danger type="primary" loading={deletion.isPending} onClick={() => void submit()}>确认删除</Button></Space>}>
  <p>仅可删除未提交的草稿。删除操作会留下审计记录。</p>
  <Input.TextArea value={reason} onChange={event => setReason(event.target.value)} maxLength={255} showCount rows={3} placeholder="填写删除原因" />
  {error && <Alert type="error" showIcon title={error} style={{ marginTop: 12 }} />}
 </Modal>
}
