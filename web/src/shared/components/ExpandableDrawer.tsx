import { FullscreenExitOutlined, FullscreenOutlined } from '@ant-design/icons'
import { Button, Drawer, Space, type DrawerProps } from 'antd'
import { useState } from 'react'

/** Expands a data drawer in place so its tables, forms and nested dialogs keep their state. */
export function ExpandableDrawer({ open, extra, width, size, afterOpenChange, ...props }: DrawerProps) {
  const [expanded, setExpanded] = useState(false)
  const label = expanded ? '退出全屏' : '全屏查看'
  return <Drawer {...props} open={open} width={expanded ? undefined : width} size={expanded ? '100vw' : size}
    afterOpenChange={visible => {
      if (!visible) setExpanded(false)
      afterOpenChange?.(visible)
    }}
    extra={<Space wrap>{extra}<Button type="text" title={label} aria-label={label}
      icon={expanded ? <FullscreenExitOutlined /> : <FullscreenOutlined />}
      onClick={() => setExpanded(value => !value)} /></Space>} />
}
