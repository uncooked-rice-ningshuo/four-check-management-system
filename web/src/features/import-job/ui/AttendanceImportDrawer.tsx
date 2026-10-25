import type { SupportedImportType } from '../../../shared/api/inspectionImportApi'
import { ImportRollbackPanel } from './ImportRollbackPanel'
import { CheckCircleOutlined, CloudUploadOutlined, DownloadOutlined, FileExcelOutlined } from '@ant-design/icons'
import { Alert, Button, Descriptions, Drawer, Modal, Progress, Select, Space, Steps, Tag, Typography, Upload } from 'antd'
import { useMemo, useRef, useState } from 'react'
import type { InspectionContext } from '../../../entities/inspection/model/inspection'
import { ApiError } from '../../../shared/api/httpClient'
import { createIdempotencyKey } from '../../../shared/utils/idempotencyKey'
import { COLLEGE_DEPARTMENT_OPTION_ID, departmentOptionId } from '../../../entities/inspection/model/inspectionScope'
import { useConfirmImport, useImportBatch, useImportRows, useImportTemplate, useUploadAttendanceImport } from '../hooks/useImportBatch'
import { useImportContext } from '../hooks/useImportContext'
import { importErrorMessage } from '../model/importErrorMessages'
import { ImportPreviewTable, type PreviewFilter } from './ImportPreviewTable'

type AttendanceImportDrawerProps = {
  typeCode?: SupportedImportType
  open: boolean
  context: InspectionContext
  initialImportScope?: InspectionContext
  scopeOptions: {
    inspectionType: { id: string; name: string }
    semesters: { id: string; name: string }[]
    weeks: { id: string; semesterId: string; weekNo?: number; name?: string }[]
  }
  departments: { id: string; name: string }[]
  memberDepartmentId?: string
  batchId?: string
  onBatchIdChange: (batchId?: string) => void
  onClose: () => void
}

const terminalStatuses = new Set(['CONFIRMED', 'CANCELLED', 'ROLLED_BACK'])

function saveTemplate(file: Blob, label: string) {
  const url = URL.createObjectURL(file)
  const link = document.createElement('a')
  link.href = url
  link.download = `${label.replaceAll('/', '-')}导入模板.xlsx`
  link.click()
  URL.revokeObjectURL(url)
}

export function AttendanceImportDrawer({ typeCode = 'ATTENDANCE_RATE', open, context, initialImportScope, scopeOptions, departments, memberDepartmentId, batchId, onBatchIdChange, onClose }: AttendanceImportDrawerProps) {
  const label = typeCode === 'CLASSROOM_HYGIENE' ? '教室卫生' : typeCode === 'DORMITORY_HYGIENE' ? '宿舍卫生' : typeCode === 'LATE_RETURN' ? '晚归' : typeCode === 'MORNING_ATTENDANCE' ? '早锻炼/早签' : typeCode === 'EVENING_SELF_STUDY' ? '晚自习' : typeCode === 'UNATTENDED_CLASSROOM' ? '无机课堂' : '到课率'
  const [targetDepartmentId, setTargetDepartmentId] = useState(memberDepartmentId ?? initialImportScope?.departmentId ?? context.departmentId ?? (departments.some(item => item.id === COLLEGE_DEPARTMENT_OPTION_ID) ? COLLEGE_DEPARTMENT_OPTION_ID : ''))
  const [targetCalendar, setTargetCalendar] = useState({ semesterId: context.semesterId, weekId: context.weekId })
  const [selectedFile, setSelectedFile] = useState<File>()
  const [fileError, setFileError] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [filter, setFilter] = useState<PreviewFilter>('ALL')
  const [confirmOpen, setConfirmOpen] = useState(false)
  const confirmationAttempt = useRef<{ batchId: string; version: number; key: string } | null>(null)
  const [confirmError, setConfirmError] = useState('')
  const [importedRows, setImportedRows] = useState<number>()
  const [confirming, setConfirming] = useState(false)
  const [rollingBack, setRollingBack] = useState(false)
  const confirmingRef = useRef(false)
  const [modal, modalHolder] = Modal.useModal()
  const targetContext = useMemo<InspectionContext>(() => ({
    semesterId: targetCalendar.semesterId,
    weekId: targetCalendar.weekId,
    inspectionTypeId: context.inspectionTypeId,
    departmentId: targetDepartmentId,
  }), [context.inspectionTypeId, targetCalendar, targetDepartmentId])
  const targetReady = Boolean(targetContext.semesterId && targetContext.weekId && targetContext.inspectionTypeId && targetContext.departmentId)
  const template = useImportTemplate(targetContext, open && !batchId && targetReady, typeCode)
  const batch = useImportBatch(batchId)
  const rows = useImportRows(batchId, page, pageSize, filter)
  const upload = useUploadAttendanceImport()
  const confirm = useConfirmImport(batchId ?? '')
  const activeBatch = batch.data
  const availableDepartments = activeBatch?.departmentId === '' && !departments.some(item => item.id === COLLEGE_DEPARTMENT_OPTION_ID)
    ? [...departments, { id: COLLEGE_DEPARTMENT_OPTION_ID, name: '校级' }] : departments
  const memberDepartmentUnavailable = memberDepartmentId !== undefined && !availableDepartments.some(item => item.id === memberDepartmentId)
  // A resumed batch owns its scope; current list filters may have changed.
  const displayScope = activeBatch ? { ...activeBatch, departmentId: departmentOptionId(activeBatch.departmentId) } : targetContext
  const destinationQuery = { purpose: batchId ? 'view' : 'entry', semesterId: displayScope.semesterId, weekId: displayScope.weekId, departmentId: displayScope.departmentId }
  const legacyClassQueryEnabled = open && Boolean(batchId && displayScope.semesterId && displayScope.weekId && displayScope.departmentId && displayScope.classId)
  const classes = useImportContext({ ...destinationQuery, kind: 'classes', grade: displayScope.grade }, legacyClassQueryEnabled, typeCode)
  const targetOptionsFailed = legacyClassQueryEnabled && classes.isError
  const targetOptionsLoading = legacyClassQueryEnabled && classes.isLoading
  const legacyClassAvailable = !displayScope.classId || Boolean(classes.data?.items.some(item => item.id === displayScope.classId))
  const classLabel = displayScope.classId ? classes.data?.items.find(item => item.id === displayScope.classId)?.label ?? '历史班级信息不可用' : ''
  const gradeLabel = displayScope.grade ?? ''
  const clearTargetFile = () => { setSelectedFile(undefined); setFileError(''); setConfirmError('') }
  const semester = scopeOptions.semesters.find(item => item.id === displayScope.semesterId)
  const week = scopeOptions.weeks.find(item => item.id === displayScope.weekId && item.semesterId === displayScope.semesterId)
  const department = availableDepartments.find(item => item.id === displayScope.departmentId)
  const inspectionType = scopeOptions.inspectionType.id === displayScope.inspectionTypeId ? scopeOptions.inspectionType : undefined
  const scopeResolved = Boolean(semester && week && department && inspectionType && legacyClassAvailable && !targetOptionsFailed && !targetOptionsLoading)
  const scopeLabels = { inspectionType: inspectionType?.name ?? '检查类型信息不可用', semester: semester?.name ?? '未找到对应学期', week: week ? week.name ?? `第 ${week.weekNo} 周` : '未找到对应周次' }
  const completedRows = activeBatch?.status === 'ROLLED_BACK' ? undefined : importedRows ?? (activeBatch?.status === 'CONFIRMED' ? activeBatch.importedRows : undefined)

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
      const created = await upload.mutateAsync({ context: targetContext, file: selectedFile, template: template.data.template, ...(typeCode !== 'ATTENDANCE_RATE' ? { typeCode } : {}) })
      onBatchIdChange(created.id)
      setPage(1)
      setFilter('ALL')
    } catch (error) {
      setConfirmError(importErrorMessage(error instanceof ApiError ? error.code : undefined))
    }
  }

  const requestConfirm = () => {
    setConfirmError('')
    if (!activeBatch || !scopeResolved) return
    // A lost response must retry the same logical request, even if it committed.
    const attempt = confirmationAttempt.current
    if (attempt?.batchId !== activeBatch.id || attempt.version !== activeBatch.version) {
      confirmationAttempt.current = { batchId: activeBatch.id, version: activeBatch.version, key: createIdempotencyKey() }
    }
    setConfirmOpen(true)
  }

  const submitConfirmation = async () => {
    if (!activeBatch || !confirmationAttempt.current || confirmingRef.current) return
    confirmingRef.current = true
    setConfirming(true)
    setConfirmError('')
    try {
      const result = await confirm.mutateAsync({ version: confirmationAttempt.current.version, idempotencyKey: confirmationAttempt.current.key })
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

  const restartUpload = () => {
    modal.confirm({
      title: `重新上传${label}文件？`,
      content: '原批次和审计记录会保留。本次重新上传会创建新批次，请核对新的导入范围。',
      okText: '保留原批次并重新上传',
      cancelText: '返回预览',
      onOk: () => {
        setTargetDepartmentId(memberDepartmentId ?? (activeBatch ? departmentOptionId(activeBatch.departmentId) : context.departmentId || (departments.some(item => item.id === COLLEGE_DEPARTMENT_OPTION_ID) ? COLLEGE_DEPARTMENT_OPTION_ID : '')))
        if (activeBatch) setTargetCalendar({ semesterId: activeBatch.semesterId, weekId: activeBatch.weekId })
        setSelectedFile(undefined)
        setFileError('')
        setConfirmError('')
        setImportedRows(undefined)
        setConfirmOpen(false)
        setPage(1)
        setPageSize(20)
        setFilter('ALL')
        confirmationAttempt.current = null
        onBatchIdChange(undefined)
      },
    })
  }

  const finishClose = () => {
    // Keep resumable batches, but do not reopen a finished batch on the next import.
    if (importedRows !== undefined || (activeBatch && terminalStatuses.has(activeBatch.status))) onBatchIdChange(undefined)
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
  const canConfirm = scopeResolved && !batch.isError && previewReady && !hasInvalidRows && (activeBatch?.validRows ?? 0) > 0 && importedRows === undefined
  const canRestart = !batch.isError && !confirming && !confirm.isPending && (activeBatch?.status === 'PARSE_FAILED' || (previewReady && hasInvalidRows))
  const currentStep = importedRows !== undefined || activeBatch?.status === 'CONFIRMED' ? 3 : batchId ? 2 : selectedFile ? 1 : 0
  const processing = activeBatch?.status === 'UPLOADED' || activeBatch?.status === 'PARSING'

  return <>
    {modalHolder}
    <Drawer title={`批量导入${label}`} aria-label={`批量导入${label}`} size="large" open={open} onClose={closeDrawer} closable={!confirming && !rollingBack} maskClosable={!confirming && !rollingBack} keyboard={!confirming && !rollingBack} destroyOnHidden>
      <Space orientation="vertical" size="large" style={{ width: '100%' }}>
        <Steps current={currentStep} responsive items={[{ title: '准备模板' }, { title: '上传解析' }, { title: '校验预览' }, { title: '确认入库' }]} />
        <section className="import-scope-summary" aria-labelledby="import-scope-title">
          <div className="import-scope-heading">
            <Typography.Title id="import-scope-title" level={5}>本次导入范围</Typography.Title>
            <Typography.Text type="secondary">选择记录归属部门；班级和年级由 Excel 内容解析并校验。</Typography.Text>
          </div>
          <div className="import-scope-grid">
            <div className="import-scope-item"><span>检查类型</span><strong>{scopeLabels.inspectionType}</strong></div>
            <div className="import-scope-item"><span>学期</span><strong>{scopeLabels.semester}</strong></div>
            <div className="import-scope-item"><span>周次</span><strong>{scopeLabels.week}</strong></div>
            <div className="import-scope-item import-scope-department">
              <span>{memberDepartmentId !== undefined ? '导入归属部门' : '目标部门'}</span>
              {batchId
                ? <strong>{department?.name ?? '部门信息不可用'}</strong>
                : <Select
                    aria-label="导入目标部门"
                    value={targetDepartmentId || undefined}
                    disabled={memberDepartmentId !== undefined}
                    options={availableDepartments.map(department => ({ value: department.id, label: department.name }))}
                    placeholder="请选择本次导入归属部门"
                    showSearch
                    optionFilterProp="label"
                    onChange={value => { setTargetDepartmentId(value); clearTargetFile() }}
                  />}
            </div>
          </div>
          {memberDepartmentUnavailable && !batchId && <Alert type="warning" title="当前账号所属部门不在可选导入归属范围，请联系管理员。" />}
          {batchId && (displayScope.classId || displayScope.grade) && <Typography.Paragraph type="secondary">历史批次保留的导入范围：{gradeLabel || '不限年级'}{displayScope.classId ? `；${classLabel}` : '；不限班级'}</Typography.Paragraph>}
          {targetOptionsFailed && <Alert type="error" title="历史批次班级信息加载失败，请重试" action={<Button onClick={() => { void classes.refetch() }}>重试班级信息</Button>} />}
        </section>

        {!batchId && <>
          <div className="import-template-action">
            <div><Typography.Title level={5}>1. 使用当前标准模板</Typography.Title><Typography.Text type="secondary">第 4 行为填写示例，不参与导入；从第 5 行填写真实数据。模板版本由服务端校验。</Typography.Text></div>
            <Button icon={<DownloadOutlined />} loading={template.isLoading} disabled={!template.data || memberDepartmentUnavailable} onClick={() => template.data && saveTemplate(template.data.file, label)}>下载标准模板</Button>
          </div>
          {template.isError && <Alert type="error" showIcon title={template.error instanceof ApiError && template.error.status === 403 ? '当前类型或导入归属部门无权限，请联系管理员' : '无法获取当前导入模板，请稍后重试'} />}
          <Upload.Dragger
            aria-label={`选择${label} XLSX 文件`}
            accept=".xlsx"
            maxCount={1}
            showUploadList={false}
            disabled={upload.isPending}
            beforeUpload={chooseFile}
          >
            <p className="ant-upload-drag-icon"><FileExcelOutlined /></p>
            <p className="ant-upload-text">选择或拖入已填写的{label}模板</p>
            <p className="ant-upload-hint">仅支持无宏的 .xlsx 文件，最多 1000 行；服务端会再次检查文件签名、模板版本和数据范围。</p>
          </Upload.Dragger>
          {fileError && <Alert type="warning" showIcon title={fileError} />}
          {selectedFile && <Alert type="success" showIcon title={selectedFile.name} description={`${Math.max(1, Math.ceil(selectedFile.size / 1024))} KB，等待上传解析`} />}
          <Button aria-label={upload.isPending ? '正在上传并解析' : '上传并生成预览'} type="primary" icon={<CloudUploadOutlined />} block loading={upload.isPending} disabled={memberDepartmentUnavailable || !targetReady || !selectedFile || !template.data || upload.isPending} onClick={startUpload}>
            {upload.isPending ? '正在上传并解析' : '上传并生成预览'}
          </Button>
        </>}

        {processing && <div role="status"><Typography.Text>文件正在解析和校验，请勿关闭页面。</Typography.Text><Progress percent={60} status="active" showInfo={false} /></div>}
        {batch.isError && <Alert type="error" showIcon title="无法读取导入批次，请检查权限或稍后重试" />}
        {activeBatch?.status === 'PARSE_FAILED' && <Alert type="error" showIcon title={importErrorMessage(activeBatch.failureCode)} description={activeBatch.failureMessage} />}
        {activeBatch && !scopeResolved && <Alert type="error" showIcon title="批次范围信息不可用，请刷新页面后核对" />}
        {canRestart && <Button onClick={restartUpload}>重新上传文件</Button>}
        {confirmError && <Alert type="error" showIcon title={confirmError} />}
        {completedRows !== undefined && <Alert type="success" showIcon icon={<CheckCircleOutlined />} title={`已成功导入 ${completedRows} 条草稿记录`} description="记录来源已标记为 Excel 导入，可返回列表继续检查和提交。" />}

        {activeBatch && <ImportRollbackPanel key={activeBatch.id} batch={activeBatch} onBusyChange={setRollingBack} />}

        {previewReady && <>
          <Descriptions bordered size="small" column={{ xs: 1, sm: 2, lg: 4 }} items={[
            { key: 'status', label: '批次状态', children: <Tag color={hasInvalidRows ? 'warning' : 'success'}>{hasInvalidRows ? '存在错误' : '校验通过'}</Tag> },
            { key: 'template', label: '模板版本', children: `v${activeBatch.templateVersion}` },
            { key: 'total', label: '总行数', children: activeBatch.totalRows },
            { key: 'valid', label: '有效 / 错误', children: `${activeBatch.validRows} / ${activeBatch.invalidRows}` },
          ]} />
          {hasInvalidRows && <Alert type="warning" showIcon title="存在错误行，当前批次不能确认" description="请根据行级建议修正文件后重新上传。" />}
          <ImportPreviewTable
            typeCode={typeCode}
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
      title={`确认写入${label}草稿`}
      open={confirmOpen}
      okText="确认写入草稿记录"
      cancelText="返回预览"
      confirmLoading={confirm.isPending}
      closable={!confirming}
      maskClosable={!confirming}
      keyboard={!confirming}
      cancelButtonProps={{ disabled: confirming }}
      okButtonProps={{ disabled: confirming }}
      onOk={submitConfirmation}
      onCancel={() => setConfirmOpen(false)}
    >
      {activeBatch && <Descriptions column={1} bordered size="small" items={[
        { key: 'type', label: '检查类型', children: scopeLabels.inspectionType },
        { key: 'term', label: '学期 / 周次', children: `${scopeLabels.semester} / ${scopeLabels.week}` },
        { key: 'department', label: '部门', children: department?.name ?? '部门信息不可用' },
        ...(activeBatch.grade || activeBatch.classId ? [
          { key: 'legacyGrade', label: '历史目标年级', children: activeBatch.grade || '不限年级' },
          { key: 'legacyClass', label: '历史目标班级', children: activeBatch.classId ? classLabel : '不限班级' },
        ] : []),
        { key: 'template', label: '模板版本', children: `v${activeBatch.templateVersion}` },
        { key: 'count', label: '将写入', children: `${activeBatch.validRows} 条草稿记录` },
      ]} />}
    </Modal>
  </>
}
