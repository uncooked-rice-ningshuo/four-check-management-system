// Field metadata describes confirmed contracts; a catalog entry alone does not grant access.
export type InspectionTypeCode = 'ATTENDANCE_RATE' | 'UNATTENDED_CLASSROOM' | 'CLASSROOM_HYGIENE' | 'MORNING_ATTENDANCE' | 'DORMITORY_HYGIENE' | 'LATE_RETURN' | 'PUBLIC_DORMITORY_HYGIENE' | 'EVENING_SELF_STUDY'
export type HygieneResult = 'EXCELLENT' | 'QUALIFIED' | 'UNQUALIFIED'
export type InspectionField = {
  key: string; label: string; kind: 'string' | 'uuid' | 'uuid[]' | 'date' | 'time' | 'number' | 'integer'; required: boolean
  maxLength?: number; minimum?: number; exclusiveMinimum?: boolean; maximum?: number
  minItems?: number; enum?: string[]; requiredWhen?: string; constValue?: number
}
export type InspectionDefinition = {
  code: InspectionTypeCode; name: string; extensionTable: string; granularity: string
  templateVersion: number; templateStatus: string; fields: InspectionField[]
  duplicateKey: string[]; plannedActions: string[]; availableActions: string[]; rules: string[]
}
// Kept in parity with the server catalog by types.test.ts. This is metadata, not form validation.
const catalog: InspectionDefinition[] = [
  {
    "code": "EVENING_SELF_STUDY",
    "name": "晚自习",
    "extensionTable": "evening_self_study_details",
    "granularity": "STUDENT_ABSENCE_EVENT",
    "templateVersion": 1,
    "templateStatus": "ACTIVE",
    "fields": [
      { "key": "studentId", "label": "学生", "kind": "uuid", "required": true },
      { "key": "classId", "label": "班级", "kind": "uuid", "required": true },
      { "key": "classroomId", "label": "教室", "kind": "uuid", "required": true },
      { "key": "date", "label": "检查日期", "kind": "date", "required": true },
      { "key": "time", "label": "检查时间", "kind": "time", "required": true },
      { "key": "period", "label": "节次", "kind": "string", "required": true, "enum": ["9-10"] },
      { "key": "remark", "label": "备注", "kind": "string", "required": false, "maxLength": 500 }
    ],
    "duplicateKey": ["semesterId", "weekId", "inspectionTypeId", "studentId", "occurredAt"],
    "plannedActions": ["CREATE", "LIST", "EDIT", "SOFT_DELETE", "BATCH_DELETE", "IMPORT"],
    "availableActions": ["CREATE", "LIST", "EDIT", "SOFT_DELETE", "BATCH_DELETE", "IMPORT"],
    "rules": ["Only absent students are recorded.", "Period 9-10 is server-enforced; each absence is one incident before the weekly multiplier; no course is linked."]
  },
  {
    "code": "ATTENDANCE_RATE",
    "name": "到课率",
    "extensionTable": "attendance_details",
    "granularity": "STUDENT_COURSE_EVENT",
    "templateVersion": 1,
    "templateStatus": "ACTIVE",
    "fields": [
      {
        "key": "studentId",
        "label": "学生",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "classId",
        "label": "班级",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "courseId",
        "label": "课程",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "classroomId",
        "label": "教室",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "date",
        "label": "检查日期",
        "kind": "date",
        "required": true
      },
      {
        "key": "time",
        "label": "检查时间",
        "kind": "time",
        "required": true
      },
      {
        "key": "period",
        "label": "节次",
        "kind": "string",
        "required": true,
        "maxLength": 64
      },
      {
        "key": "absenceHours",
        "label": "旷课学时",
        "kind": "number",
        "required": true,
        "minimum": 0,
        "exclusiveMinimum": true,
        "maximum": 24
      },
      {
        "key": "remark",
        "label": "备注",
        "kind": "string",
        "required": false,
        "maxLength": 500
      }
    ],
    "duplicateKey": [
      "semesterId",
      "weekId",
      "inspectionTypeId",
      "studentId",
      "courseId",
      "occurredAt",
      "period"
    ],
    "plannedActions": [
      "CREATE",
      "LIST",
      "EDIT",
      "SOFT_DELETE",
      "BATCH_DELETE",
      "IMPORT"
    ],
    "availableActions": [
      "CREATE",
      "LIST",
      "EDIT",
      "SOFT_DELETE",
      "BATCH_DELETE",
      "IMPORT"
    ],
    "rules": [
      "Preserve the existing SHA-256 key and timestamp precision.",
      "Date/time are normalized to occurredAt; trim period before hashing."
    ]
  },
  {
    "code": "UNATTENDED_CLASSROOM",
    "name": "无机课堂",
    "extensionTable": "attendance_details",
    "granularity": "STUDENT_COURSE_EVENT",
    "templateVersion": 1,
    "templateStatus": "ACTIVE",
    "fields": [
      {
        "key": "studentId",
        "label": "学生",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "classId",
        "label": "班级",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "courseId",
        "label": "课程",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "classroomId",
        "label": "教室",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "date",
        "label": "检查日期",
        "kind": "date",
        "required": true
      },
      {
        "key": "time",
        "label": "检查时间",
        "kind": "time",
        "required": true
      },
      {
        "key": "period",
        "label": "节次",
        "kind": "string",
        "required": true,
        "maxLength": 64
      },
      {
        "key": "absenceHours",
        "label": "旷课学时",
        "kind": "number",
        "required": true,
        "minimum": 0,
        "exclusiveMinimum": true,
        "maximum": 24
      },
      {
        "key": "remark",
        "label": "备注",
        "kind": "string",
        "required": false,
        "maxLength": 500
      }
    ],
    "duplicateKey": [
      "semesterId",
      "weekId",
      "inspectionTypeId",
      "studentId",
      "courseId",
      "occurredAt",
      "period"
    ],
    "plannedActions": [
      "CREATE",
      "LIST",
      "EDIT",
      "SOFT_DELETE",
      "BATCH_DELETE",
      "IMPORT"
    ],
    "availableActions": ["CREATE", "LIST", "EDIT", "SOFT_DELETE", "BATCH_DELETE", "IMPORT"],
    "rules": [
      "Course absence semantics match attendance; inspectionTypeId keeps records separate."
    ]
  },
  {
    "code": "CLASSROOM_HYGIENE",
    "name": "教室卫生",
    "extensionTable": "classroom_hygiene_details",
    "granularity": "OBJECT_INSPECTION",
    "templateVersion": 1,
    "templateStatus": "ACTIVE",
    "fields": [
      {
        "key": "classroomId",
        "label": "教室",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "areaId",
        "label": "负责区域",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "date",
        "label": "检查日期",
        "kind": "date",
        "required": true
      },
      {
        "key": "time",
        "label": "检查时间",
        "kind": "time",
        "required": false
      },
      {
        "key": "round",
        "label": "检查轮次",
        "kind": "string",
        "required": false,
        "maxLength": 64
      },
      {
        "key": "result",
        "label": "检查结果",
        "kind": "string",
        "required": true,
        "enum": [
          "EXCELLENT",
          "QUALIFIED",
          "UNQUALIFIED"
        ]
      },
      {
        "key": "responsibleClassIds",
        "label": "责任班级",
        "kind": "uuid[]",
        "required": true,
        "minItems": 1
      },
      {
        "key": "responsibleStudentIds",
        "label": "责任学生",
        "kind": "uuid[]",
        "required": false
      },
      {
        "key": "reasonCode",
        "label": "原因代码",
        "kind": "string",
        "required": false,
        "maxLength": 64,
        "requiredWhen": "result == UNQUALIFIED"
      },
      {
        "key": "description",
        "label": "情况说明",
        "kind": "string",
        "required": false,
        "maxLength": 500
      },
      {
        "key": "remark",
        "label": "备注",
        "kind": "string",
        "required": false,
        "maxLength": 500
      }
    ],
    "duplicateKey": [
      "semesterId",
      "weekId",
      "inspectionTypeId",
      "classroomId",
      "areaId",
      "date",
      "occurrenceDiscriminator"
    ],
    "plannedActions": [
      "CREATE",
      "LIST",
      "EDIT",
      "SOFT_DELETE",
      "BATCH_DELETE",
      "IMPORT"
    ],
    "availableActions": ["CREATE", "LIST", "EDIT", "SOFT_DELETE", "BATCH_DELETE", "IMPORT"],
    "rules": [
      "Exactly one of time or round is required; time is Asia/Shanghai local time.",
      "Duplicate event identity uses date plus TIME/time or ROUND/trimmed round.",
      "Result is exactly one of EXCELLENT, QUALIFIED, UNQUALIFIED.",
      "UNQUALIFIED requires a configured reasonCode.",
      "Responsibility associations must be scoped in SQL; scalar studentId/classId cannot represent multiple responsibilities.",
      "Whole-record edits require all responsibility scopes; reads must not expose unauthorized responsibility details.",
      "Duplicate records are rejected with source row feedback; changed result or responsibility uses versioned editing."
    ]
  },
  {
    "code": "MORNING_ATTENDANCE",
    "name": "早锻炼/早签",
    "extensionTable": "morning_exercise_details",
    "granularity": "STUDENT_DAY",
    "templateVersion": 1,
    "templateStatus": "ACTIVE",
    "fields": [
      {
        "key": "studentId",
        "label": "学生",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "classId",
        "label": "班级",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "date",
        "label": "检查日期",
        "kind": "date",
        "required": true
      },
      {
        "key": "absenceCount",
        "label": "缺勤次数",
        "kind": "integer",
        "required": false,
        "constValue": 1
      },
      {
        "key": "remark",
        "label": "备注",
        "kind": "string",
        "required": false,
        "maxLength": 500
      }
    ],
    "duplicateKey": [
      "semesterId",
      "weekId",
      "inspectionTypeId",
      "studentId",
      "date"
    ],
    "plannedActions": [
      "CREATE",
      "LIST",
      "EDIT",
      "SOFT_DELETE",
      "BATCH_DELETE",
      "IMPORT"
    ],
    "availableActions": ["CREATE", "LIST", "EDIT", "SOFT_DELETE", "BATCH_DELETE",
      "IMPORT"],
    "rules": [
      "Morning exercise and sign-in stay one type, without activityKind.",
      "One event per student/day; absenceCount is server-owned 1, totals are derived."
    ]
  },
  {
    "code": "DORMITORY_HYGIENE",
    "name": "宿舍卫生",
    "extensionTable": "dorm_hygiene_details",
    "granularity": "OBJECT_INSPECTION",
    "templateVersion": 1,
    "templateStatus": "ACTIVE",
    "fields": [
      {
        "key": "dormitoryId",
        "label": "寝室",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "date",
        "label": "检查日期",
        "kind": "date",
        "required": true
      },
      {
        "key": "time",
        "label": "检查时间",
        "kind": "time",
        "required": false
      },
      {
        "key": "round",
        "label": "检查轮次",
        "kind": "string",
        "required": false,
        "maxLength": 64
      },
      {
        "key": "result",
        "label": "检查结果",
        "kind": "string",
        "required": true,
        "enum": [
          "EXCELLENT",
          "QUALIFIED",
          "UNQUALIFIED"
        ]
      },
      {
        "key": "responsibleClassIds",
        "label": "责任班级",
        "kind": "uuid[]",
        "required": true,
        "minItems": 1
      },
      {
        "key": "responsibleStudentIds",
        "label": "责任学生",
        "kind": "uuid[]",
        "required": false
      },
      {
        "key": "reasonCode",
        "label": "原因代码",
        "kind": "string",
        "required": false,
        "maxLength": 64,
        "requiredWhen": "result == UNQUALIFIED"
      },
      {
        "key": "description",
        "label": "情况说明",
        "kind": "string",
        "required": false,
        "maxLength": 500
      },
      {
        "key": "remark",
        "label": "备注",
        "kind": "string",
        "required": false,
        "maxLength": 500
      }
    ],
    "duplicateKey": [
      "semesterId",
      "weekId",
      "inspectionTypeId",
      "dormitoryId",
      "date",
      "occurrenceDiscriminator"
    ],
    "plannedActions": [
      "CREATE",
      "LIST",
      "EDIT",
      "SOFT_DELETE",
      "BATCH_DELETE",
      "IMPORT"
    ],
    "availableActions": ["CREATE", "LIST", "EDIT", "SOFT_DELETE", "BATCH_DELETE", "IMPORT"],
    "rules": [
      "Exactly one of time or round is required; time is Asia/Shanghai local time.",
      "Duplicate event identity uses date plus TIME/time or ROUND/trimmed round.",
      "Result is exactly one of EXCELLENT, QUALIFIED, UNQUALIFIED.",
      "UNQUALIFIED requires a configured reasonCode.",
      "Responsibility associations must be scoped in SQL; scalar studentId/classId cannot represent multiple responsibilities.",
      "Whole-record edits require all responsibility scopes; reads must not expose unauthorized responsibility details.",
      "Duplicate records are rejected with source row feedback; changed result or responsibility uses versioned editing."
    ]
  },
  {
    "code": "LATE_RETURN",
    "name": "晚归",
    "extensionTable": "late_return_details",
    "granularity": "STUDENT_EVENT",
    "templateVersion": 1,
    "templateStatus": "ACTIVE",
    "fields": [
      {
        "key": "studentId",
        "label": "学生",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "classId",
        "label": "班级",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "dormitoryId",
        "label": "宿舍",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "date",
        "label": "晚归日期",
        "kind": "date",
        "required": true
      },
      {
        "key": "time",
        "label": "晚归时间",
        "kind": "time",
        "required": true
      },
      {
        "key": "remark",
        "label": "备注",
        "kind": "string",
        "required": false,
        "maxLength": 500
      }
    ],
    "duplicateKey": [
      "semesterId",
      "weekId",
      "inspectionTypeId",
      "studentId",
      "occurredAt"
    ],
    "plannedActions": [
      "CREATE",
      "LIST",
      "EDIT",
      "SOFT_DELETE",
      "BATCH_DELETE",
      "IMPORT"
    ],
    "availableActions": ["CREATE", "LIST", "EDIT", "SOFT_DELETE", "BATCH_DELETE",
      "IMPORT"],
    "rules": [
      "Split multiple students into individual events.",
      "Date/time normalize to occurredAt; violation count is server-owned 1."
    ]
  },
  {
    "code": "PUBLIC_DORMITORY_HYGIENE",
    "name": "公共宿舍卫生",
    "extensionTable": "public_dormitory_hygiene_details",
    "granularity": "OBJECT_INSPECTION",
    "templateVersion": 1,
    "templateStatus": "PLANNED",
    "fields": [
      {
        "key": "areaId",
        "label": "公共区域",
        "kind": "uuid",
        "required": true
      },
      {
        "key": "responsibleDormitoryIds",
        "label": "责任寝室",
        "kind": "uuid[]",
        "required": false
      },
      {
        "key": "date",
        "label": "检查日期",
        "kind": "date",
        "required": true
      },
      {
        "key": "time",
        "label": "检查时间",
        "kind": "time",
        "required": false
      },
      {
        "key": "round",
        "label": "检查轮次",
        "kind": "string",
        "required": false,
        "maxLength": 64
      },
      {
        "key": "result",
        "label": "检查结果",
        "kind": "string",
        "required": true,
        "enum": [
          "EXCELLENT",
          "QUALIFIED",
          "UNQUALIFIED"
        ]
      },
      {
        "key": "responsibleClassIds",
        "label": "责任班级",
        "kind": "uuid[]",
        "required": true,
        "minItems": 1
      },
      {
        "key": "responsibleStudentIds",
        "label": "责任学生",
        "kind": "uuid[]",
        "required": false
      },
      {
        "key": "reasonCode",
        "label": "原因代码",
        "kind": "string",
        "required": false,
        "maxLength": 64,
        "requiredWhen": "result == UNQUALIFIED"
      },
      {
        "key": "description",
        "label": "情况说明",
        "kind": "string",
        "required": false,
        "maxLength": 500
      },
      {
        "key": "remark",
        "label": "备注",
        "kind": "string",
        "required": false,
        "maxLength": 500
      }
    ],
    "duplicateKey": [
      "semesterId",
      "weekId",
      "inspectionTypeId",
      "areaId",
      "date",
      "occurrenceDiscriminator"
    ],
    "plannedActions": [
      "CREATE",
      "LIST",
      "EDIT",
      "SOFT_DELETE",
      "BATCH_DELETE",
      "IMPORT"
    ],
    "availableActions": ["CREATE", "LIST", "EDIT", "SOFT_DELETE", "BATCH_DELETE"],
    "rules": [
      "Exactly one of time or round is required; time is Asia/Shanghai local time.",
      "Duplicate event identity uses date plus TIME/time or ROUND/trimmed round.",
      "Result is exactly one of EXCELLENT, QUALIFIED, UNQUALIFIED.",
      "UNQUALIFIED requires a configured reasonCode.",
      "Responsibility associations must be scoped in SQL; scalar studentId/classId cannot represent multiple responsibilities.",
      "Whole-record edits require all responsibility scopes; reads must not expose unauthorized responsibility details.",
      "Duplicate records are rejected with source row feedback; changed result or responsibility uses versioned editing.",
      "Public area identity is distinct from the responsible dormitory."
    ]
  }
]

/** Returns independent metadata copies so consumers cannot mutate the shared contract. */
export function getInspectionDefinitions(): InspectionDefinition[] { return structuredClone(catalog) }
/** Unknown types must be handled as validation failures, never as a fallback schema. */
export function getInspectionDefinition(code: string): InspectionDefinition {
  const definition = catalog.find(item => item.code === code)
  if (!definition) throw new Error('VALIDATION_FAILED')
  return structuredClone(definition)
}
