import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { HygieneSituationSnippets } from './HygieneSituationSnippets'

describe('HygieneSituationSnippets', () => {
  it('shows dictionary text as a button that fills the editable field', () => {
    const onChoose = vi.fn()
    render(<HygieneSituationSnippets items={[{ id: 'floor', label: '地面不洁' }]} onChoose={onChoose} />)

    fireEvent.click(screen.getByRole('button', { name: '地面不洁' }))

    expect(onChoose).toHaveBeenCalledWith('地面不洁')
  })

  it('keeps free-text entry available when the dictionary cannot load', () => {
    render(<HygieneSituationSnippets failed onChoose={() => undefined} />)

    expect(screen.getByText('短语暂时无法加载，可直接填写情况说明。')).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})
