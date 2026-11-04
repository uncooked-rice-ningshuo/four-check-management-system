import { PlusCircleOutlined } from '@ant-design/icons'
import { Button, Space, Typography } from 'antd'
import type { InspectionOption } from '../api/inspectionOptionsApi'

type Props = {
  items?: InspectionOption[]
  loading?: boolean
  failed?: boolean
  onChoose: (text: string) => void
}

/** Presents dictionary phrases as editable text starters, never as a saved classification. */
export function HygieneSituationSnippets({ items = [], loading = false, failed = false, onChoose }: Props) {
  return (
    <section className="hygiene-situation-snippets" aria-label="常用情况说明短语">
      <div className="hygiene-situation-snippets__header">
        <Space size={7} className="hygiene-situation-snippets__title">
          <PlusCircleOutlined aria-hidden="true" />
          <Typography.Text strong>常用短语</Typography.Text>
        </Space>
        <Typography.Text type="secondary">点击后追加到说明，可继续编辑</Typography.Text>
      </div>
      {failed ? (
        <Typography.Text className="hygiene-situation-snippets__message" type="secondary">短语暂时无法加载，可直接填写情况说明。</Typography.Text>
      ) : (
        <Space wrap size={[8, 8]} className="hygiene-situation-snippets__options">
          {items.map((item) => (
            <Button className="hygiene-situation-snippets__option" key={item.id} size="small" loading={loading} aria-label={item.label} onClick={() => onChoose(item.label)}>
              {item.label}
            </Button>
          ))}
        </Space>
      )}
    </section>
  )
}
