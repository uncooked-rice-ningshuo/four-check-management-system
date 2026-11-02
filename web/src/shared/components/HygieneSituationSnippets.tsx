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
    <div className="hygiene-situation-snippets">
      <Typography.Text type="secondary">
        常用情况说明：点击短语填入文本框，之后可以继续修改。
      </Typography.Text>
      {failed ? (
        <Typography.Text type="secondary">短语暂时无法加载，可直接填写情况说明。</Typography.Text>
      ) : (
        <Space wrap size={[8, 8]}>
          {items.map((item) => (
            <Button key={item.id} size="small" loading={loading} aria-label={item.label} onClick={() => onChoose(item.label)}>
              {item.label}
            </Button>
          ))}
        </Space>
      )}
    </div>
  )
}
