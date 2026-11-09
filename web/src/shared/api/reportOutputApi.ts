import { apiRequest } from './httpClient'
export type IntroductionLineSpacingMode = 'single' | 'oneHalf' | 'double' | 'fixed'
export type IntroductionFooterLayout = 'stacked' | 'inline' | 'textOnly' | 'numberOnly' | 'custom'
export type IntroductionDocument = { type: 'doc'; titleTemplate?: string; attachmentPosition?: number; manualAttachmentList?: boolean; headerText?: string; footerText?: string; footerLayout?: IntroductionFooterLayout; footerAlign?: 'left' | 'center' | 'right'; content: Array<{ type: 'paragraph'; attrs: { style: string; textAlign: string; indent?: number; spaceBefore?: number; spaceAfter?: number; lineSpacing?: number; lineSpacingMode?: IntroductionLineSpacingMode }; content?: Array<{ type: 'text'; text: string; marks?: Array<{ type: 'bold' | 'italic' | 'strike' | 'underline' } | { type: 'textStyle'; attrs: { fontFamily?: string; fontSize?: string } }> }> }> }
export type ReportOutputSettings = { introduction: string; introductionDocument?: IntroductionDocument; wordTypeCodes: string[]; excelTypeCodes: string[]; version: number }
export type ReportOutputType = { code: string; name: string; title: string; columns: string[]; sections?: { title: string; columns: string[] }[]; excelColumns: string[] }
export type ReportOutputEditorSettings = ReportOutputSettings & { inspectionTypes: ReportOutputType[] }
export type ReportOutputSelection = { format: 'WORD' | 'EXCEL'; typeCodes: string[]; combined: boolean; settingsVersion?: number }
export type ReportOutputWeekDate = { semesterId: string; semesterName: string; weekId: string; weekNo: number; issueDate: string; version: number }
const root = (semesterId: string, weekId: string, versionNo: number) => `/reports/workflow/${encodeURIComponent(semesterId)}/${encodeURIComponent(weekId)}/versions/${versionNo}/outputs`
export const reportOutputApi = {
  settings: () => apiRequest<ReportOutputSettings>('/reports/output-settings'),
  editorSettings: () => apiRequest<ReportOutputEditorSettings>('/reports/output-settings/editor'),
  updateSettings: (input: ReportOutputSettings) => apiRequest<ReportOutputSettings>('/reports/output-settings', { method: 'PUT', body: JSON.stringify(input) }),
  weekDates: () => apiRequest<{ items: ReportOutputWeekDate[] }>('/reports/output-settings/week-dates'),
  saveWeekDate: (input: ReportOutputWeekDate) => apiRequest<ReportOutputWeekDate>(`/reports/output-settings/week-dates/${encodeURIComponent(input.semesterId)}/${encodeURIComponent(input.weekId)}`, { method: 'PUT', body: JSON.stringify({ issueDate: input.issueDate, version: input.version }) }),
  previewWord: (introductionDocument: IntroductionDocument, typeCodes: string[], period?: { semesterId: string; weekId: string }) => apiRequest<Blob>('/reports/output-settings/preview', { method: 'POST', body: JSON.stringify({ introductionDocument, typeCodes, ...period }) }, 'blob'),
  previewExcel: (typeCodes: string[]) => apiRequest<Blob>('/reports/output-settings/preview-excel', { method: 'POST', body: JSON.stringify({ typeCodes }) }, 'blob'),
  generate: (semesterId: string, weekId: string, versionNo: number, input: ReportOutputSelection) => apiRequest<Blob>(root(semesterId, weekId, versionNo), { method: 'POST', body: JSON.stringify(input) }, 'blob'),
}
