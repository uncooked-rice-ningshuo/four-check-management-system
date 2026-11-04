import { InfoCircleOutlined } from '@ant-design/icons'

type Props = {
  editing: boolean
  administratorOwned: boolean
}

/** Explains administrator-owned inspection records and preserved department attribution. */
export function InspectionAttributionNote({ editing, administratorOwned }: Props) {
  const note = !editing
    ? { title: '管理员独立记录', description: '无需选择目标部门；记录不进入部门提交、审核和通报流程。' }
    : administratorOwned
      ? { title: '管理员独立记录', description: '此记录不归属部门，也不进入部门提交、审核和通报流程。' }
      : { title: '保留原部门归属', description: '此次修改不会改变记录原有的部门归属。' }

  return (
    <div className="inspection-attribution-note" role="note">
      <InfoCircleOutlined aria-hidden="true" />
      <span className="inspection-attribution-note__copy">
        <span className="inspection-attribution-note__title">{note.title}</span>
        <span className="inspection-attribution-note__description">{note.description}</span>
      </span>
    </div>
  )
}
