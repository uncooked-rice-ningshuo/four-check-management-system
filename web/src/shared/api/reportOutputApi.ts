import { apiDownload, apiRequest } from './httpClient'
export type IntroductionLineSpacingMode = 'single' | 'oneHalf' | 'double' | 'fixed'
export type IntroductionFooterLayout = 'stacked' | 'inline' | 'textOnly' | 'numberOnly' | 'custom'
export type IntroductionDocument = { type: 'doc'; titleTemplate?: string; attachmentPosition?: number; manualAttachmentList?: boolean; headerText?: string; footerText?: string; footerLayout?: IntroductionFooterLayout; footerAlign?: 'left' | 'center' | 'right'; content: Array<{ type: 'paragraph'; attrs: { style: string; textAlign: string; indent?: number; spaceBefore?: number; spaceAfter?: number; lineSpacing?: number; lineSpacingMode?: IntroductionLineSpacingMode }; content?: Array<{ type: 'text'; text: string; marks?: Array<{ type: 'bold' | 'italic' | 'strike' | 'underline' } | { type: 'textStyle'; attrs: { fontFamily?: string; fontSize?: string } }> }> }> }
export type ReportOutputSettings = { introduction: string; introductionDocument?: IntroductionDocument; wordTypeCodes: string[]; excelTypeCodes: string[]; version: number }
export type ReportOutputType = { code: string; name: string; title: string; columns: string[]; excelColumns: string[] }
export type ReportOutputEditorSettings = ReportOutputSettings & { inspectionTypes: ReportOutputType[] }
export type ReportOutputFile = { id: string; reportVersionId: string; format: 'WORD' | 'EXCEL'; typeCodes: string[]; combined: boolean; status: 'PENDING' | 'AVAILABLE' | 'FAILED'; failureCode?: string; settingsVersion?: number; layoutVersion?: string; issueDate?: string; createdAt: string }
export type ReportOutputSelection = { format: 'WORD' | 'EXCEL'; typeCodes: string[]; combined: boolean; settingsVersion?: number }
export type ReportOutputWeekDate = { semesterId: string; semesterName: string; weekId: string; weekNo: number; issueDate: string; version: number }
const root = (semesterId: string, weekId: string, versionNo: number) => `/reports/workflow/${encodeURIComponent(semesterId)}/${encodeURIComponent(weekId)}/versions/${versionNo}/outputs`
export const reportOutputApi = {
  settings: () => apiRequest<ReportOutputSettings>('/reports/output-settings'),
  editorSettings: () => apiRequest<ReportOutputEditorSettings>('/reports/output-settings/editor'),
  updateSettings: (input: ReportOutputSettings) => apiRequest<ReportOutputSettings>('/reports/output-settings', { method: 'PUT', body: JSON.stringify(input) }),
  weekDates: () => apiRequest<{ items: ReportOutputWeekDate[] }>('/reports/output-settings/week-dates'),
  saveWeekDate: (input: ReportOutputWeekDate) => apiRequest<ReportOutputWeekDate>(`/reports/output-settings/week-dates/${encodeURIComponent(input.semesterId)}/${encodeURIComponent(input.weekId)}`, { method: 'PUT', body: JSON.stringify({ issueDate: input.issueDate, version: input.version }) }),
  previewWord: (introductionDocument: IntroductionDocument, typeCodes: string[]) => apiRequest<Blob>('/reports/output-settings/preview', { method: 'POST', body: JSON.stringify({ introductionDocument, typeCodes }) }, 'blob'),
  previewExcel: (typeCodes: string[]) => apiRequest<Blob>('/reports/output-settings/preview-excel', { method: 'POST', body: JSON.stringify({ typeCodes }) }, 'blob'),
  list: (semesterId: string, weekId: string, versionNo: number) => apiRequest<{ items: ReportOutputFile[] }>(root(semesterId, weekId, versionNo)),
  generate: (semesterId: string, weekId: string, versionNo: number, input: ReportOutputSelection) => apiRequest<ReportOutputFile>(root(semesterId, weekId, versionNo), { method: 'POST', body: JSON.stringify(input) }),
  download: (semesterId: string, weekId: string, versionNo: number, outputId: string) => apiDownload(`${root(semesterId, weekId, versionNo)}/${encodeURIComponent(outputId)}/download`),
}
