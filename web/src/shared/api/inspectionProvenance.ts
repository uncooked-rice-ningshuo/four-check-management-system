export type InspectionProvenance = {
  createdByName?: string
  submittedByName?: string
  submittedAt?: string
  canDelete?: boolean
}

/** Optional fields keep older API responses readable while rejecting malformed new metadata. */
export function validInspectionProvenance(value: Record<string, unknown>) {
  return ['createdByName', 'submittedByName', 'submittedAt'].every(key => value[key] === undefined || typeof value[key] === 'string')
    && (value.canDelete === undefined || typeof value.canDelete === 'boolean')
}
