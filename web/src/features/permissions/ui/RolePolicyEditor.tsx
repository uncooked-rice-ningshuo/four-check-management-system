import { useState } from 'react'
import { Button, DatePicker, InputNumber, Popover, Select, Typography } from 'antd'
import dayjs from 'dayjs'
import zhCN from 'antd/es/date-picker/locale/zh_CN'
import './role-policy.css'
import type { RolePolicy } from '../../../entities/organization/model/organization'

const labels: Record<string, string> = { MEMBER: '委员', MINISTER: '部长', VICE_MINISTER: '副部长', PRESIDIUM: '主席团', TEACHER: '老师' }

// Keep the shared term beside its role column; individual actions have no term editor.
export function RolePolicyEditor({ policies, roleCode, moduleKey, onChange, disabled = false }: { policies: RolePolicy[]; roleCode: string; moduleKey: string; onChange: (policies: RolePolicy[]) => void; disabled?: boolean }) {
 const [open, setOpen] = useState(false)
 const [draft, setDraft] = useState<{ validityYears: number; endsAt?: string }>({ validityYears: 0 })
 const policy = policies.find(item => item.roleCode === roleCode)
 if (!policy) return null
 const term = policy.moduleValidity?.find(item => item.moduleKey === moduleKey) ?? policy
 const patch = (value: { validityYears: number; endsAt?: string }) => onChange(policies.map(item => item.roleCode === roleCode ? { ...item, moduleValidity: [...(item.moduleValidity ?? []).filter(entry => entry.moduleKey !== moduleKey), { moduleKey, ...value }] } : item))
 const editing = open ? draft : term
 const summary = term.endsAt ? `至 ${dayjs(term.endsAt).subtract(1, 'millisecond').format('YYYY-MM-DD')}` : term.validityYears ? `授予后 ${term.validityYears} 年` : '无截止日期'
 return <Popover open={open} onOpenChange={next => { if (next) setDraft({ validityYears: term.validityYears, endsAt: term.endsAt }); setOpen(next) }} trigger="click" title={`${labels[roleCode]} · 本模块有效期`} content={<div className="role-policy-form">
   <Typography.Text type="secondary">适用于本模块全部权限，仅影响后续授予</Typography.Text>
   <label className="role-policy-field">期限类型<Select aria-label={`${labels[roleCode]}默认有效期`} value={editing.endsAt ? 'date' : editing.validityYears === 0 ? 'unlimited' : 'duration'} style={{ width: '100%' }} disabled={disabled} onChange={value => setDraft(value === 'unlimited' ? { validityYears: 0 } : value === 'duration' ? { validityYears: 1 } : { validityYears: 0, endsAt: dayjs().add(1, 'year').add(1, 'day').startOf('day').toISOString() })} options={[{ value: 'duration', label: '授予后按年有效' }, { value: 'date', label: '固定截止日期' }, { value: 'unlimited', label: '无截止日期' }]} /></label>
   {editing.endsAt ? <label className="role-policy-field">截止日期<DatePicker style={{ width: '100%' }} locale={zhCN} aria-label={`${labels[roleCode]}模板截止日期`} value={dayjs(editing.endsAt).subtract(1, 'millisecond')} disabled={disabled} allowClear={false} onChange={date => setDraft({ validityYears: 0, endsAt: date?.add(1, 'day').startOf('day').toISOString() })} /></label> : editing.validityYears > 0 && <label className="role-policy-field">有效年数<InputNumber style={{ width: '100%' }} suffix="年" aria-label={`${labels[roleCode]}有效年数`} min={1} max={100} value={editing.validityYears} disabled={disabled} onChange={value => setDraft({ validityYears: value ?? 1 })} /></label>}
   <Typography.Text type="secondary">确定后，点击页面“保存修改”生效。</Typography.Text>
   <div className="role-policy-actions"><Button size="small" onClick={() => setOpen(false)}>取消</Button><Button size="small" type="primary" disabled={disabled} onClick={() => { patch(draft); setOpen(false) }}>确定</Button></div>
 </div>}><Button type="link" size="small" disabled={disabled} aria-label={`${labels[roleCode]}有效期：${summary}`} style={{ display: 'block', marginInline: 'auto', fontWeight: 400 }}>{summary}</Button></Popover>
}
