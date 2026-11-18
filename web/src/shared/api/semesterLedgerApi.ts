import { apiRequest } from './httpClient'
import { badResponse, object, queryString } from './inspectionOptionsApi'

export type LedgerFilter = { search: string; grade: string; major: string; class: string }
export const emptyLedgerFilter: LedgerFilter = { search: '', grade: '', major: '', class: '' }
const filterBody = (filter: LedgerFilter) => ({ method: 'POST', body: JSON.stringify(filter) })

export type LedgerStudent = { number: string; name: string; className: string; grade: string; sources: { id: string; date: string; week: number; version: number; code: string; amount: number | null }[]; values: Record<string, number | null> }

export async function readSemesterLedger(semesterId: string, filter: LedgerFilter, page: number, pageSize: number) {
  const data = await apiRequest<unknown>('/discipline/ledger/search?' + queryString({ semesterId, page, pageSize }), filterBody(filter))
  if (!object(data) || !Array.isArray(data.semesters) || !Array.isArray(data.organizations) || !Array.isArray(data.columns) || !Array.isArray(data.items)
    || !Array.isArray(data.weeks) || !data.weeks.every(Number.isInteger) || typeof data.total !== 'number'
    || (data.ruleVersion !== null && typeof data.ruleVersion !== 'number') || typeof data.unassigned !== 'number' || data.page !== page || data.pageSize !== pageSize) throw badResponse()
  const organizations = data.organizations.map((o: unknown) => {
    if (!object(o) || typeof o.grade !== 'string' || typeof o.major !== 'string' || typeof o.class !== 'string' || typeof o.classId !== 'string') throw badResponse()
    return { grade: o.grade, major: o.major, class: o.class, classId: o.classId }
  })
  const semesters = data.semesters.map((s: unknown) => {
    if (!object(s) || typeof s.id !== 'string' || typeof s.name !== 'string') throw badResponse()
    return { value: s.id, label: s.name }
  })
  const columns = data.columns.map((c: unknown) => {
    if (!object(c) || typeof c.code !== 'string' || typeof c.name !== 'string' || typeof c.unit !== 'string' || (c.section !== 0 && c.section !== 1)) throw badResponse()
    if (!Array.isArray(c.members)) throw badResponse()
    const members = c.members.map((m: unknown) => {
      if (!object(m) || typeof m.code !== 'string' || typeof m.name !== 'string' || typeof m.factor !== 'number' || (m.operator !== '+' && m.operator !== '-')) throw badResponse()
      return { code: m.code, name: m.name, factor: m.factor, operator: m.operator }
    })
    return { code: c.code, name: c.name, unit: c.unit, section: c.section, members }
  })
  const items = data.items.map((r: unknown): LedgerStudent => {
    if (!object(r) || typeof r.number !== 'string' || typeof r.name !== 'string' || typeof r.className !== 'string' || typeof r.grade !== 'string' || !Array.isArray(r.sources) || !object(r.values)) throw badResponse()
    const values: LedgerStudent['values'] = {}
    for (const [key, n] of Object.entries(r.values)) {
      if (n !== null && typeof n !== 'number') throw badResponse()
      values[key] = n
    }
    const sources = r.sources.map((source: unknown) => {
      if (!object(source) || typeof source.id !== 'string' || typeof source.date !== 'string' || typeof source.week !== 'number' || typeof source.version !== 'number'
        || typeof source.code !== 'string' || (source.amount !== null && typeof source.amount !== 'number')) throw badResponse()
      return { id: source.id, date: source.date, week: source.week, version: source.version, code: source.code, amount: source.amount }
    })
    return { sources, number: r.number, name: r.name, className: r.className, grade: r.grade, values }
  })
  return { organizations, semesters, columns, items, ruleVersion: data.ruleVersion, weeks: data.weeks, unassigned: data.unassigned, total: data.total }
}

export const downloadSemesterLedger = (semesterId: string, filter: LedgerFilter) =>
  apiRequest<Blob>('/discipline/ledger/export?' + queryString({ semesterId }), filterBody(filter), 'blob')
