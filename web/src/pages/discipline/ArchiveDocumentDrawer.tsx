import { useState } from 'react'
import { Alert, Button, DatePicker, Form, Input, Select, Spin } from 'antd'
import { useQuery, useMutation } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { ExpandableDrawer } from '../../shared/components/ExpandableDrawer'
import { disciplineDocumentApi, type DocumentRequest, type DocumentVariant } from '../../shared/api/disciplineDocumentApi'
import { WordTemplateEditor } from '../../features/reporting/ui/WordTemplateEditor'
import { disciplineFields } from '../../features/reporting/model/disciplineFields'
import { fillDocumentFields } from './disciplineTemplatePreview'

export function ArchiveDocumentDrawer({id,onClose}:{id:string;onClose:()=>void}) {
  const [form,setForm] = useState<DocumentRequest>(() => {
    const today = new Intl.DateTimeFormat('sv-SE',{timeZone:'Asia/Shanghai'}).format(new Date())
    return {decisionDate:today,issueDate:today,issueNumber:''}
  })
  const [requested,setRequested] = useState(form)
  const preview = useQuery({queryKey:['discipline-document-preview',id,requested],queryFn:()=>disciplineDocumentApi.preview(id,requested),retry:false})
  const result = preview.data
  const [selection,setSelection]=useState<DocumentVariant>('letterhead')
  const [pairPreview,setPairPreview]=useState<'letterhead'|'plain'>('letterhead')
  const marked=result?.document.content.some(node=>node.attrs?.letterhead)
  const variant=marked?selection:undefined
  const plainPreview=variant==='plain' || (variant==='both' && pairPreview==='plain')
  const filenames={both:'处分决定书（两种版本）.zip',letterhead:'处分决定书（带红头版）.docx',plain:'处分决定书（无红头版）.docx'}
  const dirty = form !== requested
  const download = useMutation({mutationFn:(chosen:DocumentVariant|undefined)=>disciplineDocumentApi.download(id,{...requested,variant:chosen,templateId:result?.templateId}),onSuccess:(blob,chosen)=>{
    const url=URL.createObjectURL(blob)
    const link=document.createElement('a')
    link.href=url
    link.download=chosen?filenames[chosen]:'处分决定书.docx'
    link.click()
    window.setTimeout(()=>URL.revokeObjectURL(url),60000)
  }})
  return <ExpandableDrawer title="生成处分文书" className="report-intro-drawer" size={1100} open onClose={onClose}
    extra={<Button type="primary" disabled={!result || dirty || preview.isFetching || preview.isError || !!result.missing.length} loading={download.isPending} onClick={()=>download.mutate(variant)}>{variant==='both'?'下载两份 Word（ZIP）':'确认并下载 Word'}</Button>}>
    <Form aria-label="出文信息" layout="inline" className="archive-document-form">
      <Form.Item label="模板版本"><Select aria-label="模板版本" value={form.templateId ?? result?.templateId} options={result?.editions.map(e=>({value:e.id,label:'第 '+e.revision+' 版'}))} onChange={templateId=>setForm({...form,templateId})}/></Form.Item>
      <Form.Item label="文书确认日期"><DatePicker aria-label="文书确认日期" allowClear={false} value={dayjs(form.decisionDate)} onChange={date=>date && setForm({...form,decisionDate:date.format('YYYY-MM-DD')})}/></Form.Item>
      <Form.Item label="印发日期"><DatePicker aria-label="印发日期" allowClear={false} value={dayjs(form.issueDate)} onChange={date=>date && setForm({...form,issueDate:date.format('YYYY-MM-DD')})}/></Form.Item>
      <Form.Item label="发文字号"><Input aria-label="发文字号" maxLength={120} value={form.issueNumber} onChange={event=>setForm({...form,issueNumber:event.target.value})}/></Form.Item>
      <Button disabled={!dirty} onClick={()=>{download.reset();setRequested(form)}}>更新预览</Button>
    </Form>
    {marked && !dirty && <Form aria-label="导出版式" layout="inline">
      <Form.Item label="生成版本"><Select aria-label="生成版本" value={selection} onChange={setSelection} options={[{value:'letterhead',label:'仅带红头版'},{value:'plain',label:'仅无红头版'},{value:'both',label:'两种版本'}]}/></Form.Item>
      {variant==='both' && <Form.Item label="当前预览"><Select aria-label="当前预览" value={pairPreview} onChange={setPairPreview} options={[{value:'letterhead',label:'带红头版'},{value:'plain',label:'无红头版'}]}/></Form.Item>}
    </Form>}
    {preview.isError ? <Alert type="error" title="文书预览读取失败" description={preview.error.message} action={<Button onClick={()=>void preview.refetch()}>重试</Button>}/> :
      preview.isPending ? <Spin/> : dirty ? <Alert type="info" title="出文信息已更改，请更新预览"/> :
      result && (result.missing.length ? <Alert type="warning" title="当前模板缺少出文数据" description={result.missing.map(key=>disciplineFields[key] ?? key).join('、')}/> :
        <div className={plainPreview?'discipline-preview-plain':undefined}><WordTemplateEditor key={JSON.stringify([requested,result.templateId])} context="discipline" types={[]} readOnly value={fillDocumentFields(result.document,result.fields)} onChange={()=>{}}/></div>)}
    {download.isError && <Alert type="error" title="文书下载失败" description={download.error.message}/>}
  </ExpandableDrawer>
}
