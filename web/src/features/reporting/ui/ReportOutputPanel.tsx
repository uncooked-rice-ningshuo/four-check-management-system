import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Alert, Button, Modal, Radio, Select, Space, Typography, message } from 'antd'
import { FileExcelOutlined, FileWordOutlined } from '@ant-design/icons'
import type { ReportVersion } from '../../../entities/reporting/model/reporting'
import { reportOutputApi, type ReportOutputSelection } from '../../../shared/api/reportOutputApi'

const supported = new Set(['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'MORNING_ATTENDANCE', 'EVENING_SELF_STUDY', 'DORMITORY_HYGIENE', 'PUBLIC_DORMITORY_HYGIENE', 'LATE_RETURN', 'CLASSROOM_HYGIENE'])

export function ReportOutputPanel({ version, semesterId, weekId, canGenerate = false }: { version: ReportVersion; semesterId: string; weekId: string; canGenerate?: boolean }) {
  const [messageApi, contextHolder] = message.useMessage()
  const [activeFormat, setActiveFormat] = useState<'WORD' | 'EXCEL' | null>(null)
  const [selection, setSelection] = useState<ReportOutputSelection | null>(null)
  const mayGenerate = canGenerate && !version.content.scopeLimited
  const settings = useQuery({ queryKey: ['report-output-settings'], queryFn: reportOutputApi.settings, enabled: activeFormat !== null && mayGenerate })
  const options = [...new Map<string, { value: string; label: string }>(version.content.rows.flatMap(row => row.inspectionTypeCode && supported.has(row.inspectionTypeCode)
    ? [[row.inspectionTypeCode, { value: row.inspectionTypeCode, label: row.inspectionType }] as const]
    : [])).values()]
  const availableCodes = new Set(options.map(option => option.value))
  const configuredCodes = ((activeFormat === 'WORD' ? settings.data?.wordTypeCodes : settings.data?.excelTypeCodes) ?? []).filter(code => availableCodes.has(code))
  const defaultCodes = configuredCodes.length ? configuredCodes : options.map(option => option.value)
  const current = selection ?? { format: activeFormat ?? 'WORD', typeCodes: defaultCodes, combined: true }
  const setCurrent = (patch: Partial<ReportOutputSelection>) => setSelection({ ...current, ...patch })
  const generate = useMutation({
    mutationFn: (input: ReportOutputSelection) => reportOutputApi.generate(semesterId, weekId, version.versionNo, input),
    onSuccess: (blob, input) => {
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `通报-V${version.versionNo}.${input.format === 'WORD' ? 'docx' : 'xlsx'}`
      document.body.append(anchor)
      anchor.click()
      anchor.remove()
      window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
      setActiveFormat(format => format === input.format ? null : format)
      void messageApi.success('文件已生成并下载')
    },
    onError: error => { void messageApi.error(error.message) },
  })
  const open = (format: 'WORD' | 'EXCEL') => {
    setSelection(null)
    setActiveFormat(format)
  }
  const move = (index: number, offset: number) => {
    const codes = [...current.typeCodes]
    const next = index + offset
    if (next < 0 || next >= codes.length) return
    ;[codes[index], codes[next]] = [codes[next], codes[index]]
    setCurrent({ typeCodes: codes })
  }
  const formatLabel = activeFormat === 'WORD' ? 'Word' : 'Excel'

  if (!mayGenerate || availableCodes.size === 0) return null

  return <>
    {contextHolder}
    <Space className="report-output-actions" size={8} wrap>
      <Button icon={<FileWordOutlined aria-hidden="true" />} onClick={() => open('WORD')}>生成 Word</Button>
      <Button icon={<FileExcelOutlined aria-hidden="true" />} onClick={() => open('EXCEL')}>生成 Excel</Button>
    </Space>
    <Modal className="report-output-dialog" title={'V' + version.versionNo + ' · ' + formatLabel + ' 文件'} open={activeFormat !== null}
      width={640} destroyOnHidden onCancel={() => setActiveFormat(null)}
      onOk={() => generate.mutate({ ...current, settingsVersion: settings.data?.version })} okText={'生成并下载 ' + formatLabel} confirmLoading={generate.isPending}
      okButtonProps={{ disabled: !mayGenerate || settings.isPending || settings.isError || !current.typeCodes.length || current.typeCodes.some(code => !availableCodes.has(code)) }}
      cancelText="关闭">
      <div className="report-output-dialog-content">
        <Typography.Text type="secondary">文件只读取 V{version.versionNo} 已保存的数据；生成后直接下载，不保存文件记录，也不改变通报状态。</Typography.Text>
        {mayGenerate && <>
          {settings.isError && <Alert type="error" showIcon title="输出默认设置加载失败" action={<Button onClick={() => void settings.refetch()}>重试</Button>} />}
          <div className="report-output-field">
            <Typography.Text strong>文件内容</Typography.Text>
            <Radio.Group aria-label="文件内容" value={current.combined ? 'combined' : 'single'}
              onChange={event => setCurrent({ combined: event.target.value === 'combined', typeCodes: event.target.value === 'single' ? current.typeCodes.slice(0, 1) : defaultCodes })}
              options={[{ label: '组合文件', value: 'combined' }, { label: '单项文件', value: 'single' }]} />
          </div>
          <div className="report-output-field">
            <Typography.Text strong>检查类型</Typography.Text>
            <Select aria-label="检查类型" mode={current.combined ? 'multiple' : undefined} style={{ width: '100%' }}
              value={current.combined ? current.typeCodes : current.typeCodes[0]}
              options={options} onChange={value => setCurrent({ typeCodes: Array.isArray(value) ? value : [value] })}
              placeholder="选择检查类型" />
          </div>
          {current.combined && current.typeCodes.length > 1 && <div className="report-output-field">
            <Typography.Text strong>文件顺序</Typography.Text>
            <div className="report-output-order">{current.typeCodes.map((code, index) => {
              const name = options.find(option => option.value === code)?.label ?? code
              return <div className="report-output-order-row" key={code}>
                <span>{index + 1}. {name}</span>
                <Space size={4}>
                  <Button size="small" type="text" aria-label={'上移 ' + name} disabled={index === 0} onClick={() => move(index, -1)}>上移</Button>
                  <Button size="small" type="text" aria-label={'下移 ' + name} disabled={index === current.typeCodes.length - 1} onClick={() => move(index, 1)}>下移</Button>
                </Space>
              </div>
            })}</div>
          </div>}
        </>}
      </div>
    </Modal>
  </>
}
