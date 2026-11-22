import { disciplineTemplatePreview } from './disciplineTemplatePreview'
import { useState } from 'react'
import { Alert, Button, Empty, Modal, Popconfirm, Radio, Select, Space, Typography } from 'antd'
import '../admin/department-defaults.css'
import './DisciplineDocumentPage.css'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { disciplineDocumentApi, type DisciplineTemplate } from '../../shared/api/disciplineDocumentApi'
import { ExpandableDrawer } from '../../shared/components/ExpandableDrawer'
import { WordTemplateEditor } from '../../features/reporting/ui/WordTemplateEditor'
import type { WordTemplateDocument } from '../../shared/api/reportOutputApi'

const blank:WordTemplateDocument={type:'doc',schemaVersion:2,orientation:'portrait',content:[{type:'paragraph',attrs:{style:'body',textAlign:'left'}}]}
export function DisciplineDocumentPage(){
  const client=useQueryClient()
  const query=useQuery({queryKey:['discipline-document-templates'],queryFn:disciplineDocumentApi.list})
  const [draft,setDraft]=useState<DisciplineTemplate>()
  const [source,setSource]=useState<DisciplineTemplate>()
  const [target, setTarget] = useState<DisciplineTemplate>()
  const [preview, setPreview] = useState<DisciplineTemplate>()
  const [reuse, setReuse] = useState(false)
  const items = query.data?.items ?? []
  const sources = items.filter(item => item.templateId && item.document &&
    (item.groupKey !== target?.groupKey || item.tierCode !== target?.tierCode))
  const groups = new Map<string, DisciplineTemplate[]>()
  for (const item of items) {
    const key = JSON.stringify([item.ruleId, item.groupKey])
    const tiers = groups.get(key)
    if (tiers) tiers.push(item)
    else groups.set(key, [item])
  }
  const needsEdit = !!source && templateSignature(draft?.document) === templateSignature(source.document)
  const save = useMutation({mutationFn:disciplineDocumentApi.save,onSuccess:async()=>{
    setDraft(undefined)
    await client.invalidateQueries({queryKey:['discipline-document-templates']})
  }})
  function configure(item: DisciplineTemplate) {
    save.reset()
    setSource(undefined)
    setReuse(false)
    if (item.revision) setDraft({...item, sourceTemplateId:undefined})
    else setTarget(item)
  }
  function startEditor() {
    if (!target || (reuse && !source?.document)) return
    setDraft({...target, sourceTemplateId:source?.templateId, document:structuredClone(source?.document ?? blank)})
    setTarget(undefined)
  }

  return <section className="admin-page organization-config-page">
    <header className="page-heading"><div>
      <Typography.Title level={2}>处分文件配置</Typography.Title>
      <Typography.Paragraph>按已发布规则的累计项和处分档位配置模板。启用后的版本用于该档位出文。</Typography.Paragraph>
    </div></header>
    {query.isError ? <Alert type="error" showIcon title="模板加载失败" action={<Button onClick={() => void query.refetch()}>重试</Button>} />
      : query.isPending ? <div role="status">正在加载处分模板…</div>
      : !items.length ? <Empty description="暂无可配置的处分档位，请先发布含累计项和处分档位的规则。" />
      : <div className="department-defaults-surface document-template-list">
          <p className="department-defaults-count document-template-count">{groups.size} 个累计项 · {items.length} 个档位</p>
          <div className="department-defaults-table-scroll" role="region" aria-label="档位模板列表" tabIndex={0}>
            <table className="department-defaults-table"><thead><tr>
              <th scope="col" className="document-group-cell">累计项</th><th scope="col" className="document-group-cell">已配置</th><th scope="col">处分档位</th><th scope="col">模板版本</th><th scope="col" style={{textAlign:'right'}}>操作</th>
            </tr></thead>{Array.from(groups, ([key, tiers]) => <tbody key={key}>
              {tiers.map((item, index) => <tr key={item.tierCode ?? item.groupKey}>
                {index === 0 && <>
                  <th scope="rowgroup" rowSpan={tiers.length} className="document-group-cell">{item.name}</th>
                  <td rowSpan={tiers.length} className="document-group-cell">{tiers.filter(tier => tier.revision > 0).length} / {tiers.length} 档</td>
                </>}
                <th scope="row">{item.tierName || item.tierCode || '默认档位'}</th>
                <td>{item.revision ? '第 '+item.revision+' 版' : '待配置'}</td>
                <td><div className="department-defaults-row-actions">
                  <Button type="link" onClick={() => setPreview(item)}>预览</Button>
                  <Button type="link" onClick={() => configure(item)}>{item.revision ? '编辑模板' : '配置模板'}</Button>
                </div></td>
              </tr>)}
            </tbody>)}</table>
          </div>
      </div>}
    <Modal title="选择模板来源" open={!!target} onCancel={()=>setTarget(undefined)}
      okText="进入编辑" cancelText="取消" okButtonProps={{disabled:reuse && !source}} onOk={startEditor}>
      <Space orientation="vertical" size="middle" style={{width:'100%'}}>
        <Typography.Text>{[target?.name,target?.tierName].filter(Boolean).join(' · ')}</Typography.Text>
        <Radio.Group value={reuse} onChange={event=>{setReuse(event.target.value);setSource(undefined)}}>
          <Radio value={false}>使用空白模板</Radio>
          <Radio value={true} disabled={!sources.length}>复用已有模板</Radio>
        </Radio.Group>
        {reuse && <Select aria-label="基于已有模板编辑" placeholder="选择已配置的累计项和档位" style={{width:'100%'}}
          showSearch optionFilterProp="label" value={source?.templateId}
          options={sources.map(item=>({value:item.templateId,label:[item.name,item.tierName,'第 '+item.revision+' 版'].filter(Boolean).join(' · ')}))}
          onChange={id=>setSource(sources.find(item=>item.templateId===id))}/>}
      </Space>
    </Modal>
    <ExpandableDrawer className="report-intro-drawer word-preview-drawer" size={1100} title={[preview?.name,preview?.tierName,'模板预览'].filter(Boolean).join(' · ')}
      open={!!preview} destroyOnHidden onClose={()=>setPreview(undefined)}>
      {preview && (preview.revision && !preview.document ? <Alert type="error" title="当前版本缺少模板正文"/> :
        <section className="report-template-preview" aria-label="处分文件示例预览">
          <p className="discipline-preview-caption">{preview.revision ? '第 '+preview.revision+' 版' : '待配置 · 示例版式'} · 使用虚构学生和处分数据演示出文效果</p>
          <WordTemplateEditor key={JSON.stringify([preview.ruleId,preview.groupKey,preview.tierCode])} readOnly types={[]} context="discipline" value={disciplineTemplatePreview(preview)} onChange={()=>{}}/>
        </section>)}
    </ExpandableDrawer>
    <ExpandableDrawer className="report-intro-drawer" size={1100} destroyOnHidden title={[draft?.name,draft?.tierName,'处分文件模板'].filter(Boolean).join(' · ')} open={!!draft} onClose={()=>setDraft(undefined)} extra={<Popconfirm okText="启用" cancelText="取消" title="保存并启用此版模板？" description="该档位后续出文将使用新版式" onConfirm={()=>draft && !needsEdit && save.mutate({...draft,document:draft.document??blank})}><Button type="primary" disabled={needsEdit} loading={save.isPending}>保存并启用</Button></Popconfirm>}>
      {save.isError&&<Alert type="error" title={save.error.message}/>}
      {needsEdit && <Alert type="info" role="status" title="请先修改正文、字段或版式，再启用本档模板"/>}
      {draft&&<WordTemplateEditor key={JSON.stringify([draft.ruleId,draft.groupKey,draft.tierCode])} types={[]} context="discipline" value={draft.document??blank} onChange={document=>setDraft({...draft,document})}/>}
    </ExpandableDrawer>
  </section>
}

// Tiptap materializes paragraph defaults on the first transaction. They do not count as edits.
function templateSignature(doc?:WordTemplateDocument) {
  if(!doc)return ''
  const defaults={style:'body',textAlign:'left',indent:0,spaceBefore:0,spaceAfter:0,lineSpacing:0,lineSpacingMode:'fixed',border:null}
  return JSON.stringify({...doc,orientation:doc.orientation??'portrait',content:doc.content.map(n=>
    n.type==='paragraph'?{...n,attrs:{...defaults,...n.attrs},content:n.content??[]}:n
  )},(_key,value)=>value && typeof value==='object' && !Array.isArray(value)
    ?Object.fromEntries(Object.entries(value).sort(([a],[b])=>a.localeCompare(b))):value)
}
