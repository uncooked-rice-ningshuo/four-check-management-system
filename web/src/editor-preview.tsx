import React, { useState } from 'react'
import { createRoot } from 'react-dom/client'
import { Button } from 'antd'
import { ReportIntroductionEditor } from './features/reporting/ui/ReportIntroductionEditor'
import type { IntroductionDocument } from './shared/api/reportOutputApi'
import './shared/styles/global.css'

const initial: IntroductionDocument = { type: 'doc', content: [
  { type: 'paragraph', attrs: { style: 'heading', textAlign: 'left' }, content: [{ type: 'text', text: '本周日常管理情况' }] },
  { type: 'paragraph', attrs: { style: 'body', textAlign: 'justify' }, content: [{ type: 'text', text: '根据本周日常检查记录，现将有关情况通报如下。请各部门及时核对，并按要求完成后续工作。' }] },
  { type: 'paragraph', attrs: { style: 'body', textAlign: 'justify' }, content: [{ type: 'text', text: '各项检查数据以已确认记录为准，具体情况见附件。' }] },
] }

function Preview() {
  const [document, setDocument] = useState(initial)
  return <main style={{ marginLeft: 255 }}><section className="report-output-settings-page"><section className="report-output-editor-section"><ReportIntroductionEditor value={document} onChange={setDocument} toolbarAction={<Button type="primary">保存正文</Button>} /></section></section></main>
}

createRoot(document.getElementById('root')!).render(<React.StrictMode><Preview /></React.StrictMode>)
