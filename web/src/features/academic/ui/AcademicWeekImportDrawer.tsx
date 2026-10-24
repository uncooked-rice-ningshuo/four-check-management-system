import { DownloadOutlined, FileExcelOutlined, UploadOutlined } from '@ant-design/icons'
import { Alert, Button, Descriptions, Drawer, Modal, Select, Space, Table, Tag, Typography, Upload, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useRef, useState } from 'react'
import type { AcademicCalendar } from '../../../entities/academic/model/academic'
import type { AcademicWeekImportBatch, AcademicWeekImportPreviewRow } from '../../../entities/academic/model/academicWeekImport'
import { createIdempotencyKey } from '../../../shared/utils/idempotencyKey'
import { ApiError } from '../../../shared/api/httpClient'
import { useConfirmAcademicWeekImport, useAcademicWeekImportSpec, useAcademicWeekImportTemplate, usePreviewAcademicWeekImport } from '../hooks/useAcademicWeekImport'

type AcademicWeekImportDrawerProps = {
  open: boolean
  calendar: AcademicCalendar
  onClose: () => void
}

type ConfirmAttempt = { batchId: string; version: number; key: string }

function saveTemplate(file: Blob) {
  const url = URL.createObjectURL(file)
  const link = document.createElement('a')
  link.href = url
  link.download = '周次导入模板.xlsx'
  link.click()
  URL.revokeObjectURL(url)
}

function requestError(error: unknown) {
  return error instanceof ApiError ? error.message : '请求失败，请稍后重试'
}

export function AcademicWeekImportDrawer({ open, calendar, onClose }: AcademicWeekImportDrawerProps) {
  const [semesterId, setSemesterId] = useState('')
  const [selectedFile, setSelectedFile] = useState<File>()
  const [batch, setBatch] = useState<AcademicWeekImportBatch>()
  const [fileError, setFileError] = useState('')
  const [requestErrorText, setRequestErrorText] = useState('')
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [confirmedRows, setConfirmedRows] = useState<number>()
  const confirmationAttempt = useRef<ConfirmAttempt | null>(null)
  const spec = useAcademicWeekImportSpec(open)
  const template = useAcademicWeekImportTemplate(open)
  const preview = usePreviewAcademicWeekImport()
  const confirm = useConfirmAcademicWeekImport(batch?.batchId ?? '')
  const [modal, modalHolder] = Modal.useModal()

  const finishClose = () => {
    setSemesterId('')
    setSelectedFile(undefined)
    setBatch(undefined)
    setFileError('')
    setRequestErrorText('')
    setConfirmOpen(false)
    setConfirmedRows(undefined)
    confirmationAttempt.current = null
    onClose()
  }

  const selectedSemester = calendar.semesters.find(item => item.id === semesterId)
  const selectedYear = calendar.academicYears.find(item => item.id === selectedSemester?.academicYearId)
  const targetIsDisabled = Boolean(selectedSemester && (!selectedSemester.enabled || !selectedYear?.enabled))
  const semesterOptions = calendar.semesters.map(semester => {
    const year = calendar.academicYears.find(item => item.id === semester.academicYearId)
    const disabled = !semester.enabled || !year?.enabled
    return { value: semester.id, label: `${year?.name ?? '学年信息不可用'} · ${semester.name}${disabled ? '（停用）' : ''}` }
  })
  const rows = batch?.rows ?? []
  const rowsColumns: ColumnsType<AcademicWeekImportPreviewRow> = [
    { title: 'Excel 行', dataIndex: 'rowNo', width: 90, fixed: 'left' },
    { title: '周次', width: 100, render: (_, row) => row.values.week_no || '—' },
    { title: '开始日期', width: 130, render: (_, row) => row.values.start_date || '—' },
    { title: '结束日期', width: 130, render: (_, row) => row.values.end_date || '—' },
    { title: '倍率', width: 90, render: (_, row) => row.values.attendance_penalty_multiplier?.trim() || '1' },
    { title: '备注', dataIndex: ['values', 'note'], width: 220, ellipsis: true, render: value => value || '—' },
    { title: '校验结果', width: 320, render: (_, row) => row.errors.length ? <Typography.Text type="danger">{row.errors.map(item => item.message).join('；')}</Typography.Text> : <Tag color="success">通过</Tag> },
  ]

  const chooseFile = (file: File) => {
    if (!file.name.toLowerCase().endsWith('.xlsx')) {
      setSelectedFile(undefined)
      setBatch(undefined)
      setFileError('仅支持无宏的 .xlsx 文件')
      confirmationAttempt.current = null
      return Upload.LIST_IGNORE
    }
    setSelectedFile(file)
    setBatch(undefined)
    setConfirmedRows(undefined)
    setFileError('')
    setRequestErrorText('')
    confirmationAttempt.current = null
    return false
  }

  const selectSemester = (value: string) => {
    setSemesterId(value)
    setBatch(undefined)
    setConfirmedRows(undefined)
    setRequestErrorText('')
    confirmationAttempt.current = null
  }

  const replaceFile = () => {
    setSelectedFile(undefined)
    setBatch(undefined)
    setConfirmedRows(undefined)
    setFileError('')
    setRequestErrorText('')
    confirmationAttempt.current = null
  }

  const uploadForPreview = async () => {
    if (!semesterId || !selectedFile || preview.isPending) return
    setRequestErrorText('')
    try {
      const next = await preview.mutateAsync({ semesterId, file: selectedFile })
      setBatch(next)
      setConfirmedRows(undefined)
      confirmationAttempt.current = null
    } catch (error) {
      setRequestErrorText(requestError(error))
    }
  }

  const requestConfirm = () => {
    if (!batch || batch.status !== 'PREVIEW_READY' || batch.invalidRows > 0 || batch.validRows === 0 || confirm.isPending) return
    const current = confirmationAttempt.current
    if (current?.batchId !== batch.batchId || current.version !== batch.version) {
      confirmationAttempt.current = { batchId: batch.batchId, version: batch.version, key: createIdempotencyKey() }
    }
    setConfirmOpen(true)
  }

  const submitConfirmation = async () => {
    const current = confirmationAttempt.current
    if (!batch || !current || confirm.isPending) return
    setRequestErrorText('')
    try {
      const result = await confirm.mutateAsync({ version: current.version, idempotencyKey: current.key })
      setConfirmedRows(result.importedRows)
      setConfirmOpen(false)
      confirmationAttempt.current = null
      void message.success(`已导入 ${result.importedRows} 个周次`)
    } catch (error) {
      // Keep the same key after an uncertain response so a retry is safe after commit.
      setRequestErrorText(requestError(error))
      setConfirmOpen(false)
    }
  }

  const close = () => {
    if (batch && batch.status !== 'CONFIRMED' && confirmedRows === undefined) {
      modal.confirm({
        title: '当前周次预览尚未确认',
        content: '关闭后预览批次和源文件会保留在审计记录中，重新打开时需要再次选择目标并上传。',
        okText: '保留并关闭',
        cancelText: '继续处理',
        onOk: finishClose,
      })
      return
    }
    finishClose()
  }

  const impactsToday = Boolean(batch?.affectsToday)
  const confirmAvailable = batch?.status === 'PREVIEW_READY' && batch.invalidRows === 0 && batch.validRows > 0 && confirmedRows === undefined

  return <>
    {modalHolder}
    <Drawer title="批量导入周次" aria-label="批量导入周次" size="large" open={open} onClose={close} destroyOnHidden>
      <Space orientation="vertical" size="large" style={{ width: '100%' }}>
        <section aria-labelledby="academic-week-import-target">
          <Typography.Title id="academic-week-import-target" level={5}>1. 选择目标学期</Typography.Title>
          {calendar.semesters.length === 0
            ? <Alert type="warning" showIcon title="当前没有可选学期，请先创建学年和学期。" />
            : <Select
                aria-label="导入目标学期"
                style={{ width: '100%' }}
                value={semesterId || undefined}
                placeholder="请选择本次新增周次所属学期"
                options={semesterOptions}
                showSearch
                optionFilterProp="label"
                onChange={selectSemester}
              />}
          {selectedSemester && targetIsDisabled && <Alert style={{ marginTop: 12 }} type="warning" showIcon title="目标学期或所属学年已停用" description="仍可维护该学期周次；停用期间这些周次不会参与自动当前工作周识别。" />}
        </section>

        <section aria-labelledby="academic-week-import-template">
          <Typography.Title id="academic-week-import-template" level={5}>2. 下载并填写标准模板</Typography.Title>
          <Space wrap>
            <Button icon={<DownloadOutlined />} loading={template.isLoading} disabled={!template.data} onClick={() => template.data && saveTemplate(template.data.blob)}>下载周次导入模板</Button>
            <Typography.Text type="secondary">仅支持新增；最多 60 行；日期须在目标学期内。</Typography.Text>
          </Space>
          {template.isError && <Alert style={{ marginTop: 12 }} type="error" showIcon title="模板下载失败" action={<Button onClick={() => void template.refetch()}>重试</Button>} />}
          {spec.data && <div className="academic-week-import-rules">
            {spec.data.fields.map(field => <Typography.Paragraph key={field.code} type="secondary" style={{ marginBottom: 4 }}><strong>{field.header}</strong>：{field.format}</Typography.Paragraph>)}
          </div>}
          {spec.isError && <Alert style={{ marginTop: 12 }} type="warning" showIcon title="导入规则暂时不可用，请仍使用本页下载的当前模板。" />}
        </section>

        {!batch && <section aria-labelledby="academic-week-import-upload">
          <Typography.Title id="academic-week-import-upload" level={5}>3. 上传并预览</Typography.Title>
          <Upload.Dragger aria-label="选择周次 XLSX 文件" accept=".xlsx" maxCount={1} showUploadList={false} disabled={preview.isPending} beforeUpload={chooseFile}>
            <p className="ant-upload-drag-icon"><FileExcelOutlined /></p>
            <p className="ant-upload-text">选择或拖入已填写的周次模板</p>
            <p className="ant-upload-hint">服务端会再次检查文件签名、模板版本、行数和周次冲突。</p>
          </Upload.Dragger>
          {selectedFile && <Alert style={{ marginTop: 12 }} type="success" showIcon title={selectedFile.name} description={`${Math.max(1, Math.ceil(selectedFile.size / 1024))} KB`} />}
          {fileError && <Alert style={{ marginTop: 12 }} type="warning" showIcon title={fileError} />}
          <Button style={{ marginTop: 12 }} type="primary" block icon={<UploadOutlined />} loading={preview.isPending} disabled={!semesterId || !selectedFile || preview.isPending || calendar.semesters.length === 0} onClick={() => void uploadForPreview()}>
            {preview.isPending ? '正在上传并校验' : '上传并生成预览'}
          </Button>
        </section>}

        {requestErrorText && <Alert type="error" showIcon title={requestErrorText} />}

        {batch && <section aria-labelledby="academic-week-import-preview">
          <Typography.Title id="academic-week-import-preview" level={5}>4. 校验预览</Typography.Title>
          <Descriptions bordered size="small" column={{ xs: 1, sm: 2 }} items={[
            { key: 'target', label: '目标学期', children: `${batch.academicYearName} · ${batch.semesterName}` },
            { key: 'status', label: '状态', children: <Tag color={batch.invalidRows ? 'warning' : 'success'}>{batch.invalidRows ? '存在错误' : '校验通过'}</Tag> },
            { key: 'count', label: '总行数', children: `${batch.totalRows} 行` },
            { key: 'valid', label: '有效 / 错误', children: `${batch.validRows} / ${batch.invalidRows}` },
          ]} />
          {impactsToday && <Alert style={{ marginTop: 12 }} type="warning" showIcon title="导入日期覆盖今天，确认后可能改变系统自动识别的当前工作周。" description="导入不会修改已设置的临时工作周覆盖。" />}
          {batch.invalidRows > 0 && <Alert style={{ marginTop: 12 }} type="warning" showIcon title="存在错误行，整批不能确认。请修正 Excel 后重新上传。" />}
          <div style={{ overflowX: 'auto', marginTop: 16 }}>
            <Table<AcademicWeekImportPreviewRow> rowKey="rowNo" size="small" pagination={false} dataSource={rows} columns={rowsColumns} scroll={{ x: 1080 }} />
          </div>
          {confirmedRows !== undefined && <Alert style={{ marginTop: 12 }} type="success" showIcon title={`成功新增 ${confirmedRows} 个周次。`} />}
          <Space style={{ marginTop: 16 }} wrap>
            <Button disabled={confirm.isPending} onClick={replaceFile}>更换文件</Button>
            <Button type="primary" disabled={!confirmAvailable || confirm.isPending} loading={confirm.isPending} onClick={requestConfirm}>确认新增 {batch.validRows} 个周次</Button>
          </Space>
        </section>}
      </Space>
    </Drawer>

    <Modal
      title="确认新增周次"
      open={confirmOpen}
      okText="确认导入"
      cancelText="返回预览"
      confirmLoading={confirm.isPending}
      onOk={() => void submitConfirmation()}
      onCancel={() => setConfirmOpen(false)}
      destroyOnHidden
    >
      {batch && <Descriptions bordered size="small" column={1} items={[
        { key: 'semester', label: '目标学期', children: `${batch.academicYearName} · ${batch.semesterName}` },
        { key: 'weeks', label: '周次范围', children: batch.rows.map(row => row.values.week_no).join('、') },
        { key: 'count', label: '将新增', children: `${batch.validRows} 个周次` },
        { key: 'multiplier', label: '倍率', children: `${batch.rows.filter(row => row.values.attendance_penalty_multiplier?.trim() === '2').length} 个 2 倍周次，其余按 1 倍` },
        ...(impactsToday ? [{ key: 'today', label: '当前周影响', children: '日期覆盖今天，可能改变自动识别的当前工作周。' }] : []),
      ]} />}
    </Modal>
  </>
}
