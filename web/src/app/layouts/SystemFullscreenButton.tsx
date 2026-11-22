import maximize from '@iconify-icons/lucide/maximize'
import minimize from '@iconify-icons/lucide/minimize'
import { Icon } from '@iconify/react'
import { App, Tooltip } from 'antd'
import { useSyncExternalStore } from 'react'

function subscribe(onChange: () => void) {
  document.addEventListener('fullscreenchange', onChange)
  return () => document.removeEventListener('fullscreenchange', onChange)
}

const isFullscreen = () => document.fullscreenElement === document.documentElement

export function SystemFullscreenButton() {
  const active = useSyncExternalStore(subscribe, isFullscreen)
  const { message } = App.useApp()
  const label = active ? '退出系统全屏' : '进入系统全屏'

  async function toggle() {
    if (!document.fullscreenEnabled && !active) {
      message.warning('当前浏览器或嵌入页面未开放全屏功能')
      return
    }
    try {
      if (active) await document.exitFullscreen()
      else await document.documentElement.requestFullscreen()
    } catch {
      message.warning('浏览器未能切换全屏，请检查页面权限后重试')
    }
  }

  return <Tooltip title={label} trigger={['focus', 'hover']}>
    <button type="button" className="header-fullscreen" aria-label={label} onClick={() => void toggle()}>
      <Icon icon={active ? minimize : maximize} width={16} height={16} aria-hidden="true" />
    </button>
  </Tooltip>
}
