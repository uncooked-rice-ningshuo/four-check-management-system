import { InfoCircleOutlined } from '@ant-design/icons'

type Props = {
  editing: boolean
  administratorOwned: boolean
}

/** Explains administrator-owned inspection records and preserved department attribution. */
export function InspectionAttributionNote({ editing, administratorOwned }: Props) {
  const note = !editing
    ? { title: '管理员独立记录', description: '确认提交时选择负责部门或管理员通道。' }
    : administratorOwned
      ? { title: '管理员独立记录', description: '可在提交批次中查看本次提交的去向。' }
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
