import type { JSONContent } from '@tiptap/core'
import type { DisciplineTemplate } from '../../shared/api/disciplineDocumentApi'
import type { WordTemplateDocument } from '../../shared/api/reportOutputApi'

const field = (key: string): JSONContent => ({type:'templateField', attrs:{key:'discipline.' + key}})
const text = (value: string): JSONContent => ({type:'text', text:value})
const example: WordTemplateDocument = {
  type:'doc', schemaVersion:2, orientation:'portrait',
  content:[
    {type:'paragraph', attrs:{style:'title',textAlign:'center'}, content:[text('关于给予'),field('studentName'),text('同学处分的决定')]},
    {type:'paragraph', attrs:{style:'body',indent:2}, content:[field('studentName'),text('，'),field('class'),text('学生。在“'),field('group'),text('”项下累计达到 '),field('amount'),field('unit'),text('。')]},
    {type:'paragraph', attrs:{style:'body',indent:2}, content:[text('依据'),field('clause'),text('，决定给予该生'),field('tier'),text('处分。')]},
    {type:'paragraph', attrs:{style:'signature',textAlign:'right'}, content:[text('示例学院学生工作办公室')]},
    {type:'paragraph', attrs:{style:'date',textAlign:'right'}, content:[field('decisionDate')]},
  ],
}

/** Substitute fictional values in a display copy, preserving the saved document and text formatting. */
export function disciplineTemplatePreview(item: DisciplineTemplate): WordTemplateDocument {
  const values: Record<string,string> = {
    'discipline.issueNumber':'示例院发〔2026〕1号','discipline.sex':'女','discipline.enrollmentYear':'2025',
    'discipline.major':'示例专业','discipline.classNumber':'01','discipline.schoolYear':'2025—2026学年','discipline.semester':'第二学期',
    'discipline.violationCount':item.unit === '次' ? item.exampleAmount ?? '4' : '4','discipline.violationName':item.violationName?.trim() || item.name,'discipline.authorityNumber':'示例院规〔2025〕2号',
    'discipline.clauseReference':'第二章第八条','discipline.decisionDate':'二○二六年十一月二十日','discipline.issuedDate':'2026年11月23日',
    'discipline.studentName':'陈小禾','discipline.studentNumber':'DEMO-001','discipline.class':'示例一班',
    'discipline.group':item.name,'discipline.amount':item.exampleAmount ?? '4','discipline.unit':item.unit || '次',
    'discipline.tier':item.tierName || item.tierCode || '示例档位','discipline.clause':'《示例学生管理办法》第八条',
    'discipline.decidedAt':'2026年11月20日','common.generatedDate':'2026年11月23日',
  }
  return fillDocumentFields(item.document ?? example,values)
}

export function fillDocumentFields(template: WordTemplateDocument,values:Record<string,string>): WordTemplateDocument {
  const substitute = (node:JSONContent):JSONContent => {
    if (node.type === 'templateField' && values[node.attrs?.key]) {
      return {type:'text',text:values[node.attrs?.key],marks:node.marks}
    }
    return node.content ? {...node,content:node.content.map(substitute)} : node
  }
  return {...template,content:template.content.map(substitute)}
}
