import {expect,it} from 'vitest'
import {disciplineTemplatePreview} from './disciplineTemplatePreview'
import type {DisciplineTemplate} from '../../shared/api/disciplineDocumentApi'

it('按当前项和档位预览，优先使用该项配置的违纪事实名称',()=>{
  const template:DisciplineTemplate={ruleId:'rule',groupKey:'late',name:'晚归',revision:2,tierName:'严重警告',unit:'次',exampleAmount:'8',
    document:{type:'doc',orientation:'portrait',schemaVersion:2,content:[{type:'paragraph',content:
      ['group','violationName','amount','unit','tier','authorityNumber','clauseReference'].map(key=>({type:'templateField',attrs:{key:'discipline.'+key}}))}]}}
  const fields=(item:DisciplineTemplate)=>disciplineTemplatePreview(item).content[0].content?.map(run=>run.text)
  expect(fields(template)).toEqual(['晚归','晚归','8','次','严重警告','【未配置文书依据】','【未配置依据条款】'])
  expect(fields({...template,name:'到课率',violationName:'旷课',unit:'学时',exampleAmount:'20',authorityNumber:'院发〔2026〕9号',clauseReference:'第三章第二款'})).toEqual(['到课率','旷课','20','学时','严重警告','院发〔2026〕9号','第三章第二款'])
})
