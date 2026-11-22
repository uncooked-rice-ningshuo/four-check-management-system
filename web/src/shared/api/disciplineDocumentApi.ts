import { apiRequest } from './httpClient'
import type { WordTemplateDocument } from './reportOutputApi'

export type DisciplineTemplate = {ruleId:string;groupKey:string;violationName?:string;unit?:string;exampleAmount?:string;tierCode?:string;tierName?:string;templateId?:string;sourceTemplateId?:string;name:string;revision:number;document?:WordTemplateDocument}
export type DocumentRequest = {templateId?:string;decisionDate:string;issueDate:string;issueNumber:string}
export type ArchiveDocumentPreview = {document:WordTemplateDocument; templateId:string; editions:{id:string;revision:number}[];fields:Record<string,string>;missing:string[]}
const url='/discipline/document-templates'
export const disciplineDocumentApi={
  list:()=>apiRequest<{items:DisciplineTemplate[];sources?:DisciplineTemplate[]}>(url),
  save:(template:DisciplineTemplate)=>apiRequest<{saved:boolean}>(url,{method:'PUT',body:JSON.stringify(template)}),
  preview:(id:string,input:DocumentRequest)=>apiRequest<ArchiveDocumentPreview>('/discipline/archive/'+encodeURIComponent(id)+'/document/preview',{method:'POST',body:JSON.stringify(input)}),
  download:(id:string,input:DocumentRequest)=>apiRequest<Blob>('/discipline/archive/'+encodeURIComponent(id)+'/document',{method:'POST',body:JSON.stringify(input)},'blob'),
}
