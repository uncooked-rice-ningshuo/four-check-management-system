import { CheckCircleOutlined, CloudUploadOutlined, DownloadOutlined, FileExcelOutlined } from '@ant-design/icons'
import { Alert, Button, Descriptions, Drawer, Modal, Progress, Select, Space, Steps, Tag, Typography, Upload } from 'antd'
import { useMemo, useRef, useState } from 'react'
import type { InspectionContext } from '../../../entities/inspection/model/inspection'
import { ApiError } from '../../../shared/api/httpClient'
import { useConfirmImport, useImportBatch, useImportRows, useImportTemplate, useUploadAttendanceImport } from '../hooks/useImportBatch'
import { importErrorMessage } from '../model/importErrorMessages'
import { ImportPreviewTable, type PreviewFilter } from './ImportPreviewTable'

type AttendanceImportDrawerProps = {
  open: boolean
  context: InspectionContext
  scopeLabels: { inspectionType: string; semester: string; week: string }
  departments: { id: string; name: string }[]
  batchId?: string
  onBatchIdChange: (batchId?: string) => void
  onClose: () => void
}

const terminalStatuses = new Set(['CONFIRMED', 'CANCELLED'])

function saveTemplate(file: Blob) {
  const url = URL.createObjectURL(file)
  const link = document.createElement('a')
  link.href = url
  link.download = '到课率导入模板.xlsx'
  link.click()
  URL.revokeObjectURL(url)
}

export function AttendanceImportDrawer({ open, context, scopeLabels, departments, batchId, onBatchIdChange, onClose }: AttendanceImportDrawerProps) {
  const [targetDepartmentId, setTargetDepartmentId] = useState(context.departmentId)
  const [selectedFile, setSelectedFile] = useState<File>()
  const [fileError, setFileError] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [filter, setFilter] = useState<PreviewFilter>('ALL')
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [idempotencyKey, setIdempotencyKey] = useState('')
  const [confirmError, setConfirmError] = useState('')
  const [importedRows, setImportedRows] = useState<number>()
  const [confirming, setConfirming] = useState(false)
  const confirmingRef = useRef(false)
  const [modal, modalHolder] = Modal.useModal()
  const targetContext = useMemo(() => ({ ...context, departmentId: targetDepartmentId }), [context, targetDepartmentId])
  const targetReady = Boolean(targetContext.semesterId && targetContext.weekId && targetContext.inspectionTypeId && targetContext.departmentId)
  const template = useImportTemplate(targetContext, open && !batchId && targetReady)
  const batch = useImportBatch(batchId)
  const rows = useImportRows(batchId, page, pageSize, filter)
  const upload = useUploadAttendanceImport()
  const confirm = useConfirmImport(targetContext, batchId ?? '')
  const activeBatch = batch.data

  const chooseFile = (file: File) => {
    if (!file.name.toLowerCase().endsWith('.xlsx')) {
      setSelectedFile(undefined)
      setFileError('仅支持无宏的 .xlsx 文件')
      return Upload.LIST_IGNORE
    }
    setFileError('')
    setSelectedFile(file)
    return false
  }

  const startUpload = async () => {
    if (!selectedFile || !template.data) return
    setConfirmError('')
    try {
      const created = await upload.mutateAsync({ context: targetContext, file: selectedFile, template: template.data.template })
      onBatchIdChange(created.id)
      setPage(1)
      setFilter('ALL')
    } catch (error) {
      setConfirmError(importErrorMessage(error instanceof ApiError ? error.code : undefined))
    }
  }

  const requestConfirm = () => {
    setConfirmError('')
    setIdempotencyKey(crypto.randomUUID())
    setConfirmOpen(true)
  }

  const submitConfirmation = async () => {
    if (!activeBatch || confirmingRef.current) return
    confirmingRef.current = true
    setConfirming(true)
    setConfirmError('')
    try {
      const result = await confirm.mutateAsync({ version: activeBatch.version, idempotencyKey })
      setImportedRows(result.importedRows)
      setConfirmOpen(false)
    } catch (error) {
      setConfirmError(importErrorMessage(error instanceof ApiError ? error.code : undefined))
      setConfirmOpen(false)
    } finally {
      confirmingRef.current = false
      setConfirming(false)
    }
  }

  const finishClose = () => {
    setSelectedFile(undefined)
    setFileError('')
    setConfirmError('')
    setImportedRows(undefined)
    onClose()
  }

  const closeDrawer = () => {
    if (activeBatch && !terminalStatuses.has(activeBatch.status) && importedRows === undefined) {
      modal.confirm({
        title: '当前导入批次尚未确认',
        content: '关闭后批次会保留，再次打开导入抽屉可以继续查看和确认。',
        okText: '保留并关闭',
        cancelText: '继续处理',
        onOk: finishClose,
      })
      return
    }
    finishClose()
  }

  const previewReady = activeBatch?.status === 'PREVIEW_READY'
  const hasInvalidRows = (activeBatch?.invalidRows ?? 0) > 0
  const canConfirm = previewReady && !hasInvalidRows && (activeBatch?.validRows ?? 0) > 0 && importedRows === undefined
  const currentStep = importedRows !== undefined || activeBatch?.status === 'CONFIRMED' ? 3 : batchId ? 2 : selectedFile ? 1 : 0
  const processing = activeBatch?.status === 'UPLOADED' || activeBatch?.status === 'PARSING'

  return <>
    {modalHolder}
    <Drawer title="批量导入到课率" aria-label="批量导入到课率" size="large" open={open} onClose={closeDrawer} destroyOnHidden>
      <Space orientation="vertical" size="large" style={{ width: '100%' }}>
        <Steps current={currentStep} responsive items={[{ title: '准备模板' }, { title: '上传解析' }, { title: '校验预览' }, { title: '确认入库' }]} />
        <section className="import-scope-summary" aria-labelledby="import-scope-title">
          <div className="import-scope-heading">
            <Typography.Title id="import-scope-title" level={5}>本次导入范围</Typography.Title>
            <Typography.Text type="secondary">选择目标部门后，系统将按该范围校验并写入草稿。</Typography.Text>
          </div>
          <div className="import-scope-grid">
            <div className="import-scope-item"><span>检查类型</span><strong>{scopeLabels.inspectionType}</strong></div>
            <div className="import-scope-item"><span>学期</span><strong>{scopeLabels.semester}</strong></div>
            <div className="import-scope-item"><span>周次</span><strong>{scopeLabels.week}</strong></div>
            <div className="import-scope-item import-scope-department">
              <span>目标部门</span>
              {batchId
                ? <strong>{departments.find(department => department.id === (activeBatch?.departmentId || targetDepartmentId))?.name ?? '未知部门'}</strong>
                : <Select
                    aria-label="导入目标部门"
                    value={targetDepartmentId || undefined}
                    options={departments.map(department => ({ value: department.id, label: department.name }))}
                    placeholder="请选择本次导入归属部门"
                    showSearch
                    optionFilterProp="label"
                    onChange={setTargetDepartmentId}
                  />}
            </div>
          </div>
        </section>

        {!batchId && <>
          <div className="import-template-action">
            <div><Typography.Title level={5}>1. 使用当前标准模板</Typography.Title><Typography.Text type="secondary">模板版本由服务端校验，旧模板无法上传确认。</Typography.Text></div>
            <Button icon={<DownloadOutlined />} loading={template.isLoading} disabled={!template.data} onClick={() => template.data && saveTemplate(template.data.file)}>下载标准模板</Button>
          </div>
          {template.isError && <Alert type="error" showIcon title="无法获取当前导入模板，请稍后重试" />}
          <Upload.Dragger
            aria-label="选择到课率 XLSX 文件"
            accept=".xlsx"
            maxCount={1}
            showUploadList={false}
            disabled={upload.isPending}
            beforeUpload={chooseFile}
          >
            <p className="ant-upload-drag-icon"><FileExcelOutlined /></p>
            <p className="ant-upload-text">选择或拖入已填写的到课率模板</p>
            <p className="ant-upload-hint">仅支持无宏的 .xlsx 文件，最多 1000 行；服务端会再次检查文件签名、模板版本和数据范围。</p>
          </Upload.Dragger>
          {fileError && <Alert type="warning" showIcon title={fileError} />}
          {selectedFile && <Alert type="success" showIcon title={selectedFile.name} description={`${Math.max(1, Math.ceil(selectedFile.size / 1024))} KB，等待上传解析`} />}
          <Button aria-label={upload.isPending ? '正在上传并解析' : '上传并生成预览'} type="primary" icon={<CloudUploadOutlined />} block loading={upload.isPending} disabled={!targetReady || !selectedFile || !template.data || upload.isPending} onClick={startUpload}>
            {upload.isPending ? '正在上传并解析' : '上传并生成预览'}
          </Button>
        </>}

        {processing && <div role="status"><Typography.Text>文件正在解析和校验，请勿关闭页面。</Typography.Text><Progress percent={60} status="active" showInfo={false} /></div>}
        {batch.isError && <Alert type="error" showIcon title="无法读取导入批次，请检查权限或稍后重试" />}
        {activeBatch?.status === 'PARSE_FAILED' && <Alert type="error" showIcon title={importErrorMessage(activeBatch.failureCode)} description={activeBatch.failureMessage} />}
        {confirmError && <Alert type="error" showIcon title={confirmError} />}
        {importedRows !== undefined && <Alert type="success" showIcon icon={<CheckCircleOutlined />} title={`已成功导入 ${importedRows} 条草稿记录`} description="记录来源已标记为 Excel 导入，可返回列表继续检查和提交。" />}

        {previewReady && <>
          <Descriptions bordered size="small" column={{ xs: 1, sm: 2, lg: 4 }} items={[
            { key: 'status', label: '批次状态', children: <Tag color={hasInvalidRows ? 'warning' : 'success'}>{hasInvalidRows ? '存在错误' : '校验通过'}</Tag> },
            { key: 'template', label: '模板版本', children: `v${activeBatch.templateVersion}` },
            { key: 'total', label: '总行数', children: activeBatch.totalRows },
            { key: 'valid', label: '有效 / 错误', children: `${activeBatch.validRows} / ${activeBatch.invalidRows}` },
          ]} />
          {hasInvalidRows && <Alert type="warning" showIcon title="存在错误行，当前批次不能确认" description="请根据行级建议修正文件后重新上传。" />}
          <ImportPreviewTable
            rows={rows.data?.items ?? []}
            loading={rows.isLoading}
            page={rows.data?.page ?? page}
            pageSize={rows.data?.pageSize ?? pageSize}
            total={rows.data?.total ?? 0}
            filter={filter}
            onFilterChange={(nextFilter) => { setFilter(nextFilter); setPage(1) }}
            onPageChange={(nextPage, nextPageSize) => { setPage(nextPage); setPageSize(nextPageSize) }}
          />
          <Button type="primary" block disabled={!canConfirm || confirm.isPending} onClick={requestConfirm}>确认导入 {activeBatch.validRows} 条记录</Button>
        </>}
      </Space>
    </Drawer>
    <Modal
      title="确认写入到课率草稿"
      open={confirmOpen}
      okText="确认写入草稿记录"
      cancelText="返回预览"
      confirmLoading={confirm.isPending}
      okButtonProps={{ disabled: confirming }}
      onOk={submitConfirmation}
      onCancel={() => setConfirmOpen(false)}
    >
      {activeBatch && <Descriptions column={1} bordered size="small" items={[
        { key: 'type', label: '检查类型', children: activeBatch.inspectionTypeId },
        { key: 'term', label: '学期 / 周次', children: `${activeBatch.semesterId} / ${activeBatch.weekId}` },
        { key: 'department', label: '部门', children: activeBatch.departmentId },
        { key: 'template', label: '模板版本', children: `v${activeBatch.templateVersion}` },
        { key: 'count', label: '将写入', children: `${activeBatch.validRows} 条 DRAFT 草稿记录` },
      ]} />}
    </Modal>
  </>
}
