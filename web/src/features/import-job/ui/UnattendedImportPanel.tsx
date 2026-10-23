import type { ComponentProps } from 'react'
import { InspectionImportScopePanel } from './InspectionImportScopePanel'
export function UnattendedImportPanel(props: Omit<ComponentProps<typeof InspectionImportScopePanel>, 'typeCode'>) {
  return <InspectionImportScopePanel {...props} typeCode="UNATTENDED_CLASSROOM" />
}
