import { Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { ReportingRecord, ReportingRecordReference } from '../../../entities/reporting/model/reporting'
import { inspectionResultLabel } from '../model/labels'

export type ReportRecordDisplayField = { key: string; label: string; value: string }
export type ReportRecordDisplaySection = { key: string; title: string; fields: ReportRecordDisplayField[] }

const emptyValue = '—'
const personRecordTypes = new Set(['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'EVENING_SELF_STUDY', 'MORNING_ATTENDANCE', 'LATE_RETURN'])
const hygieneRecordTypes = new Set(['DORMITORY_HYGIENE', 'CLASSROOM_HYGIENE', 'PUBLIC_DORMITORY_HYGIENE'])
const reportTypeCodesByName: Record<string, string> = {
  '到课率': 'ATTENDANCE_RATE',
  '无机课堂': 'UNATTENDED_CLASSROOM',
  '晚自习': 'EVENING_SELF_STUDY',
  '早锻炼/早签': 'MORNING_ATTENDANCE',
  '晚归': 'LATE_RETURN',
  '宿舍卫生': 'DORMITORY_HYGIENE',
  '教室卫生': 'CLASSROOM_HYGIENE',
  '公共宿舍卫生': 'PUBLIC_DORMITORY_HYGIENE',
}

export function reportRecordTypeCodeForName(inspectionTypeName?: string) {
  return inspectionTypeName ? reportTypeCodesByName[inspectionTypeName] : undefined
}

export function reportRecordUsesPersonFilters(recordTypeCode?: string) {
  return recordTypeCode ? personRecordTypes.has(recordTypeCode) : true
}

function text(value: string | number | undefined | null) {
  return value === null || value === undefined ? '' : String(value).trim()
}

function field(key: string, label: string, value: string | number | undefined | null): ReportRecordDisplayField | undefined {
  const valueText = text(value)
  return valueText ? { key, label, value: valueText } : undefined
}

function requiredField(key: string, label: string, value: string | number | undefined | null): ReportRecordDisplayField {
  return field(key, label, value) ?? { key, label, value: emptyValue }
}

function quantity(key: string, label: string, value: number | undefined | null, unit: string): ReportRecordDisplayField | undefined {
  return value === null || value === undefined ? undefined : field(key, label, `${value} ${unit}`)
}

function checkDateTime(record: ReportingRecord) {
  // 无机课堂 counts one violation per day: the stored clock is a fixed midnight placeholder.
  if (record.typeCode === 'UNATTENDED_CLASSROOM') return text(record.details.date) || emptyValue
  return [record.details.date, record.details.time].map(text).filter(Boolean).join(' ') || emptyValue
}

function referenceValue(references: ReportingRecordReference[] | undefined) {
  return references?.map(reference => {
    const label = reference.label.trim()
    const grade = reference.grade?.trim()
    return label && grade && !label.includes(grade) ? `${label}（${grade}）` : label
  }).filter(Boolean).join('、')
}

function sourceLabel(source: string | undefined) {
  if (!source) return undefined
  const labels: Record<string, string> = {
    MANUAL: '手工录入',
    EXCEL: 'Excel 导入',
    REPORT_CORRECTION: '通报修正',
    MIGRATION: '历史迁移',
  }
  return labels[source] ?? source
}

function personInformationFields(record: ReportingRecord): ReportRecordDisplayField[] {
  return [
    requiredField('studentName', '姓名', record.studentName),
    requiredField('studentNumber', '学号', record.studentNumber),
    requiredField('className', '班级', record.className),
    requiredField('grade', '年级', record.grade),
  ]
}

function hygieneObjectFields(record: ReportingRecord): ReportRecordDisplayField[] {
  const fields: Array<ReportRecordDisplayField | undefined> = []
  switch (record.typeCode) {
    case 'DORMITORY_HYGIENE':
      fields.push(requiredField('dormitory', '寝室', record.currentDormitoryName))
      break
    case 'CLASSROOM_HYGIENE':
      fields.push(requiredField('classroom', '教室', record.currentClassroomName))
      break
    case 'PUBLIC_DORMITORY_HYGIENE':
      fields.push(requiredField('responsibleDormitory', '责任寝室', referenceValue(record.responsibleDormitories)))
      break
  }
  return fields.filter((item): item is ReportRecordDisplayField => Boolean(item))
}

function responsibilityFields(record: ReportingRecord): ReportRecordDisplayField[] {
  const fields: Array<ReportRecordDisplayField | undefined> = [
    field('responsibleClasses', '责任班级', referenceValue(record.responsibleClasses)),
    field('responsibleStudents', '责任学生', referenceValue(record.responsibleStudents)),
  ]
  if (record.typeCode === 'PUBLIC_DORMITORY_HYGIENE') {
    fields.push(field('responsibleDormitories', '责任寝室', referenceValue(record.responsibleDormitories)))
  }
  return fields.filter((item): item is ReportRecordDisplayField => Boolean(item))
}

function reasonFields(record: ReportingRecord): ReportRecordDisplayField[] {
  const reasonLabel = text(record.currentReasonLabel)
  const reasonCode = text(record.details.reasonCode)
  if (!reasonLabel && !reasonCode) return []
  return [
    { key: 'reason', label: '不合格原因', value: reasonLabel || reasonCode },
    ...(reasonLabel && reasonCode ? [{ key: 'reasonCode', label: '原因编码', value: reasonCode }] : []),
  ]
}

function personEventFields(record: ReportingRecord, snapshot = false): ReportRecordDisplayField[] {
  const multiplier = record.details.penaltyMultiplier === undefined
    ? snapshot ? { key: 'penaltyMultiplier', label: '适用倍数', value: '该版本未保存倍数快照' } : undefined
    : field('penaltyMultiplier', '适用倍数', `${record.details.penaltyMultiplier} 倍`)
  switch (record.typeCode) {
    case 'ATTENDANCE_RATE':
      return [
        requiredField('period', '发生节次', record.details.period),
        requiredField('absenceHours', '缺课节次数', record.details.absenceHours === undefined ? undefined : `${record.details.absenceHours} 节次`),
        multiplier,
        requiredField('penaltyHours', '违纪计入次数', record.details.penaltyHours === undefined || record.details.penaltyHours === null ? undefined : `${record.details.penaltyHours} 次`),
      ].filter((item): item is ReportRecordDisplayField => Boolean(item))
    case 'UNATTENDED_CLASSROOM':
      return [
        { key: 'basePenaltyCount', label: '单次基础次数', value: '1 次' },
        multiplier,
        requiredField('penaltyCount', '违纪计入次数', record.details.penaltyCount === undefined || record.details.penaltyCount === null ? undefined : `${record.details.penaltyCount} 次`),
      ].filter((item): item is ReportRecordDisplayField => Boolean(item))
    case 'EVENING_SELF_STUDY':
      return [
        requiredField('period', '晚自习节次', record.details.period),
        { key: 'basePenaltyCount', label: '单次基础次数', value: '1 次' },
        multiplier,
        requiredField('penaltyCount', '违纪计入次数', record.details.penaltyCount === undefined || record.details.penaltyCount === null ? undefined : `${record.details.penaltyCount} 次`),
      ].filter((item): item is ReportRecordDisplayField => Boolean(item))
    case 'MORNING_ATTENDANCE':
      return [requiredField('absenceCount', '缺勤次数', record.details.absenceCount === undefined ? undefined : `${record.details.absenceCount} 次`)]
    case 'LATE_RETURN':
      return [{ key: 'violationCount', label: '违纪计入次数', value: '1 次' }]
    default:
      return []
  }
}

function hygieneResultFields(record: ReportingRecord): ReportRecordDisplayField[] {
  return [
    requiredField('result', '检查结果', record.details.result ? inspectionResultLabel(record.details.result) : undefined),
    ...reasonFields(record),
  ]
}

function noteFields(record: ReportingRecord, includeDescription: boolean): ReportRecordDisplayField[] {
  return [
    ...(includeDescription ? [field('description', '情况说明', record.details.description)] : []),
    field('remark', '备注', record.details.remark),
    field('source', '记录来源', sourceLabel(record.source)),
  ].filter((item): item is ReportRecordDisplayField => Boolean(item))
}

function personRecordSections(record: ReportingRecord): ReportRecordDisplaySection[] {
  const dateTitle = record.typeCode === 'LATE_RETURN' ? '晚归时间'
    : record.typeCode === 'MORNING_ATTENDANCE' || record.typeCode === 'UNATTENDED_CLASSROOM' ? '检查日期'
      : '检查时间'
  const sections: ReportRecordDisplaySection[] = [
    { key: 'person', title: '基本信息', fields: personInformationFields(record) },
    { key: 'time', title: dateTitle, fields: [{ key: 'dateTime', label: dateTitle, value: checkDateTime(record) }] },
  ]

  if (record.typeCode === 'ATTENDANCE_RATE' || record.typeCode === 'UNATTENDED_CLASSROOM') {
    sections.push({ key: 'course', title: '课程与地点', fields: [
      requiredField('course', '课程', record.currentCourseName),
      requiredField('classroom', '教室', record.currentClassroomName),
    ] })
  } else if (record.typeCode === 'EVENING_SELF_STUDY') {
    sections.push({ key: 'classroom', title: '晚自习地点', fields: [requiredField('classroom', '教室', record.currentClassroomName)] })
  } else if (record.typeCode === 'LATE_RETURN') {
    sections.push({ key: 'dormitory', title: '晚归地点', fields: [requiredField('dormitory', '宿舍', record.currentDormitoryName)] })
  }

  const eventTitle: Record<string, string> = {
    ATTENDANCE_RATE: '到课情况',
    UNATTENDED_CLASSROOM: '无机课堂情况',
    EVENING_SELF_STUDY: '晚自习情况',
    MORNING_ATTENDANCE: '考勤情况',
    LATE_RETURN: '晚归情况',
  }
  sections.push({ key: 'event', title: eventTitle[record.typeCode] ?? '检查情况', fields: personEventFields(record, true) })
  const notes = noteFields(record, false)
  if (notes.length > 0) sections.push({ key: 'notes', title: '补充记录', fields: notes })
  return sections
}

function hygieneRecordSections(record: ReportingRecord): ReportRecordDisplaySection[] {
  const objectTitle = record.typeCode === 'DORMITORY_HYGIENE' ? '宿舍卫生对象'
    : record.typeCode === 'CLASSROOM_HYGIENE' ? '教室卫生对象' : '公共宿舍卫生对象'
  const objectFields = [...hygieneObjectFields(record), ...responsibilityFields(record)]
  const timeFields = [
    requiredField('dateTime', '检查日期与时间', checkDateTime(record)),
    field('round', '检查轮次', record.details.round),
  ].filter((item): item is ReportRecordDisplayField => Boolean(item))
  const notes = noteFields(record, true)
  return [
    { key: 'object', title: objectTitle, fields: objectFields },
    { key: 'time', title: '检查时间', fields: timeFields },
    { key: 'result', title: '检查结果', fields: hygieneResultFields(record) },
    ...(notes.length > 0 ? [{ key: 'notes', title: '现场记录', fields: notes }] : []),
  ]
}

function unknownRecordSections(record: ReportingRecord): ReportRecordDisplaySection[] {
  const candidates: Array<ReportRecordDisplayField | undefined> = [
    field('studentName', '姓名', record.studentName),
    field('studentNumber', '学号', record.studentNumber),
    field('className', '班级', record.className),
    field('grade', '年级', record.grade),
    field('dormitory', '宿舍', record.currentDormitoryName),
    field('course', '课程', record.currentCourseName),
    field('classroom', '教室', record.currentClassroomName),
    field('area', '检查区域', record.currentAreaName),
    field('date', '检查日期', record.details.date),
    field('time', '检查时间', record.details.time),
    field('period', '节次', record.details.period),
    field('round', '检查轮次', record.details.round),
    quantity('absenceHours', '缺课学时', record.details.absenceHours, '小时'),
    quantity('penaltyHours', '违纪计入学时或次数', record.details.penaltyHours, '小时/次'),
    quantity('penaltyMultiplier', '适用倍数', record.details.penaltyMultiplier, '倍'),
    quantity('penaltyCount', '违纪计入次数', record.details.penaltyCount, '次'),
    quantity('absenceCount', '缺勤次数', record.details.absenceCount, '次'),
    field('result', '检查结果', record.details.result),
    field('reasonCode', '原因编码', record.details.reasonCode),
    field('description', '情况说明', record.details.description),
    field('remark', '备注', record.details.remark),
    field('source', '记录来源', sourceLabel(record.source)),
    field('responsibleClasses', '责任班级', referenceValue(record.responsibleClasses)),
    field('responsibleStudents', '责任学生', referenceValue(record.responsibleStudents)),
    field('responsibleDormitories', '责任寝室', referenceValue(record.responsibleDormitories)),
  ]
  return [{ key: 'raw', title: `记录字段 · ${record.typeCode}`, fields: candidates.filter((item): item is ReportRecordDisplayField => Boolean(item)) }]
}

export function reportRecordDisplaySections(record: ReportingRecord): ReportRecordDisplaySection[] {
  if (personRecordTypes.has(record.typeCode)) return personRecordSections(record)
  if (hygieneRecordTypes.has(record.typeCode)) return hygieneRecordSections(record)
  return unknownRecordSections(record)
}

function tableValue(value: string) {
  return <span className="report-record-table-value">{value}</span>
}

function groupedTableValue(fields: ReportRecordDisplayField[]) {
  const visibleFields = fields.filter(item => item.value !== emptyValue)
  if (visibleFields.length === 0) return tableValue(emptyValue)
  return <div className="report-record-table-groups">
    {visibleFields.map(item => <span className="report-record-table-group" key={item.key}>
      <Typography.Text type="secondary">{item.label}</Typography.Text>
      <Typography.Text className="report-record-table-group-value">{item.value}</Typography.Text>
    </span>)}
  </div>
}

function group(title: string, children: ColumnsType<ReportingRecord>[number][]) {
  return { title, key: title, children }
}

function personColumns(recordTypeCode: string): ColumnsType<ReportingRecord> {
  const basicInformation = group('基本信息', [
    { title: '姓名', dataIndex: 'studentName', key: 'studentName', width: 118, render: value => tableValue(text(value) || emptyValue) },
    { title: '学号', dataIndex: 'studentNumber', key: 'studentNumber', width: 120, render: value => tableValue(text(value) || emptyValue) },
    { title: '班级', dataIndex: 'className', key: 'className', width: 165, render: value => tableValue(text(value) || emptyValue) },
    { title: '年级', dataIndex: 'grade', key: 'grade', width: 88, render: value => tableValue(text(value) || emptyValue) },
  ])
  const timeColumn = {
    title: recordTypeCode === 'LATE_RETURN' ? '晚归时间' : recordTypeCode === 'UNATTENDED_CLASSROOM' ? '检查日期' : '检查时间',
    key: 'dateTime', width: 175,
    render: (_value: unknown, record: ReportingRecord) => tableValue(checkDateTime(record)),
  }

  if (recordTypeCode === 'ATTENDANCE_RATE' || recordTypeCode === 'UNATTENDED_CLASSROOM') {
    return [basicInformation, timeColumn,
      { title: '课程与地点', key: 'courseAndClassroom', width: 240, render: (_value, record) => groupedTableValue([
        requiredField('course', '课程', record.currentCourseName), requiredField('classroom', '教室', record.currentClassroomName),
      ]) },
      { title: recordTypeCode === 'ATTENDANCE_RATE' ? '到课情况' : '课堂情况', key: 'attendanceDetails', width: 260,
        render: (_value, record) => groupedTableValue(personEventFields(record)) },
    ]
  }
  if (recordTypeCode === 'EVENING_SELF_STUDY') {
    return [basicInformation, timeColumn,
      { title: '晚自习教室', dataIndex: 'currentClassroomName', width: 170, render: value => tableValue(text(value) || emptyValue) },
      { title: '晚自习情况', key: 'eveningDetails', width: 250, render: (_value, record) => groupedTableValue(personEventFields(record)) },
    ]
  }
  if (recordTypeCode === 'MORNING_ATTENDANCE') {
    return [basicInformation,
      { title: '检查日期', key: 'date', width: 150, render: (_value, record) => tableValue(text(record.details.date) || emptyValue) },
      { title: '考勤情况', key: 'morningDetails', width: 170, render: (_value, record) => groupedTableValue(personEventFields(record)) },
    ]
  }
  return [basicInformation, timeColumn,
    { title: '晚归宿舍', dataIndex: 'currentDormitoryName', width: 190, render: value => tableValue(text(value) || emptyValue) },
    { title: '晚归情况', key: 'lateReturnDetails', width: 150, render: (_value, record) => groupedTableValue(personEventFields(record)) },
  ]
}

function hygieneColumns(recordTypeCode: string): ColumnsType<ReportingRecord> {
  const responsibility = {
    title: '责任对象', key: 'responsiblePeople', width: recordTypeCode === 'PUBLIC_DORMITORY_HYGIENE' ? 330 : 270,
    render: (_value: unknown, record: ReportingRecord) => groupedTableValue(responsibilityFields(record)),
  }
  const time = {
    title: '检查时间', key: 'hygieneDateTime', width: 190,
    render: (_value: unknown, record: ReportingRecord) => groupedTableValue([
      requiredField('dateTime', '时间', checkDateTime(record)), field('round', '轮次', record.details.round),
    ].filter((item): item is ReportRecordDisplayField => Boolean(item))),
  }
  const result = {
    title: '检查结果', key: 'hygieneResult', width: 210,
    render: (_value: unknown, record: ReportingRecord) => groupedTableValue(hygieneResultFields(record)),
  }
  const note = {
    title: '现场记录', key: 'hygieneNotes', width: 290,
    render: (_value: unknown, record: ReportingRecord) => groupedTableValue(noteFields(record, true)),
  }

  if (recordTypeCode === 'DORMITORY_HYGIENE') {
    return [group('检查对象', [
      { title: '寝室', dataIndex: 'currentDormitoryName', width: 210, render: value => tableValue(text(value) || emptyValue) },
      responsibility,
    ]), time, result, note]
  }
  if (recordTypeCode === 'CLASSROOM_HYGIENE') {
    return [group('检查对象', [
      { title: '教室', key: 'classroom', width: 220, render: (_value, record) => groupedTableValue(hygieneObjectFields(record)) },
      responsibility,
    ]), time, result, note]
  }
  return [group('检查对象', [
    { title: '责任寝室', key: 'responsibleDormitories', width: 230, render: (_value, record) => {
      const dormitoryFields = [
        requiredField('responsibleDormitories', '责任寝室', referenceValue(record.responsibleDormitories)),
        field('legacyArea', '历史检查区域', record.currentAreaName),
      ].filter((item): item is ReportRecordDisplayField => Boolean(item))
      return groupedTableValue(dormitoryFields)
    } },
    responsibility,
  ]), time, result, note]
}

function unknownColumns(): ColumnsType<ReportingRecord> {
  return [{ title: '检查时间', key: 'dateTime', width: 190, render: (_value, record) => tableValue(checkDateTime(record)) },
    { title: '记录类型', dataIndex: 'typeCode', width: 190 },
    { title: '检查结果与说明', key: 'content', render: (_value, record) => groupedTableValue(unknownRecordSections(record)[0].fields) }]
}

export function reportRecordColumns(recordTypeCode?: string): ColumnsType<ReportingRecord> {
  if (!recordTypeCode) return unknownColumns()
  if (hygieneRecordTypes.has(recordTypeCode)) return hygieneColumns(recordTypeCode)
  if (personRecordTypes.has(recordTypeCode)) return personColumns(recordTypeCode)
  return unknownColumns()
}

export function reportRecordTableWidth(columns: ColumnsType<ReportingRecord>): number {
  return columns.reduce<number>((width, column) => {
    if ('children' in column && column.children) return width + reportRecordTableWidth(column.children)
    return width + (typeof column.width === 'number' ? column.width : 0)
  }, 0 as number)
}

