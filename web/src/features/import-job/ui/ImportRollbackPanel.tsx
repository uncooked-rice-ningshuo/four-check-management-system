import { Alert, Button, Form, Input, Modal } from 'antd'
import { useRef, useState } from 'react'
import type { ImportBatch } from '../../../entities/import-job/model/importJob'
import { ApiError } from '../../../shared/api/httpClient'
import { useImportRollback } from '../hooks/useImportRollback'

export function ImportRollbackPanel({ batch, onBusyChange }: { batch: ImportBatch; onBusyChange: (busy: boolean) => void }) {
  const [selection, setSelection] = useState<{ version: number; count: number }>()
  const [error, setError] = useState('')
  const mutation = useImportRollback(batch.id)
  const pending = useRef(false)
  const submit = async ({ reason }: { reason: string }) => {
    if (!selection || pending.current) return
    pending.current = true
    onBusyChange(true)
    setError('')
    try {
      await mutation.mutateAsync({ version: selection.version, reason })
      setSelection(undefined)
    } catch (failure) {
      setError(failure instanceof ApiError && failure.status < 500 ? failure.message : '回滚失败，请稍后重试。')
    } finally {
      pending.current = false
      onBusyChange(false)
    }
  }
  return <>
    {batch.status === 'CONFIRMED' && <Button danger onClick={() => { setError(''); setSelection({ version: batch.version, count: batch.importedRows }) }}>整批回滚</Button>}
    {batch.status === 'ROLLED_BACK' && <Alert type="info" showIcon title={`已整批回滚 ${batch.importedRows} 条记录`} description="原文件、批次、解析结果及审计记录已保留。" />}
    {selection && <Modal open title="确认整批回滚" footer={null} onCancel={() => { if (!pending.current) setSelection(undefined) }} closable={!mutation.isPending} maskClosable={!mutation.isPending} keyboard={!mutation.isPending}>
      <p>将撤销本批次全部 {selection.count} 条记录。仅允许全部记录仍为未提交、未锁定且未修改的草稿；原文件和审计记录会保留。</p>
      {error && <Alert type="error" showIcon title={error} />}
      <Form layout="vertical" onFinish={submit} disabled={mutation.isPending}>
        <Form.Item name="reason" label="回滚原因" rules={[{ required: true, whitespace: true, message: '请填写回滚原因' }]}><Input.TextArea maxLength={255} /></Form.Item>
        <Button disabled={mutation.isPending} onClick={() => setSelection(undefined)}>取消</Button>{' '}
        <Button danger type="primary" htmlType="submit" loading={mutation.isPending}>确认回滚全部记录</Button>
      </Form>
    </Modal>}
  </>
}
