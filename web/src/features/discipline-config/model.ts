export type DisciplineTier = { authorityNumber:string; clauseReference:string; code: string; name: string; lower: number; upper?: number; teacherConfirmationRequired: boolean }
export type DisciplineTerm = { inspectionTypeId: string; inspectionTypeName: string; sourceUnit: string; operator: '+' | '-'; factor: number }
export type DisciplineGroup = { violationName?:string; key: string; name: string; unit: string; terms: DisciplineTerm[]; tiers: DisciplineTier[] }
export type DisciplineRuleVersion = { id: string; version: number; status: 'DRAFT' | 'PUBLISHED' | 'SUPERSEDED'; rowVersion: number; createdBy: string; createdByName?: string; createdAt: string; publishedBy?: string; publishedByName?: string; publishedAt?: string; groups: DisciplineGroup[] }
export type DisciplineValidationIssue = { path: string; code: string; message: string }

// 旧版本的草稿复制可能丢失组序；查看时以最初发布的配置顺序为准，新组保持当前版本的相对顺序。
export function orderGroupsByInitialConfiguration(groups: DisciplineGroup[], versions: DisciplineRuleVersion[]) {
  const initial = versions.filter((item) => item.status !== 'DRAFT').sort((a, b) => a.version - b.version)[0]
  if (!initial) return groups
  const positions = new Map(initial.groups.map((group, index) => [group.key, index]))
  return groups.map((group, index) => ({ group, index })).sort((a, b) =>
    (positions.get(a.group.key) ?? initial.groups.length + a.index) - (positions.get(b.group.key) ?? initial.groups.length + b.index),
  ).map(({ group }) => group)
}

export function calculateRuleExample(group: DisciplineGroup, sampleValues: Record<string, number>) {
  const total = group.terms.reduce((sum, term, index) => {
    const value = sampleValues[term.inspectionTypeId] ?? 0
    return index === 0 || term.operator === '+' ? sum + value * term.factor : sum - value * term.factor
  }, 0)
  const tier = group.tiers.find((item) => total >= item.lower && (item.upper === undefined || total < item.upper))
  return { total, tier }
}
