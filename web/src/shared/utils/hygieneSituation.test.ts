import { describe, expect, it } from 'vitest'
import { appendHygieneSituationSnippet } from './hygieneSituation'

describe('appendHygieneSituationSnippet', () => {
  it('fills an empty situation field', () => {
    expect(appendHygieneSituationSnippet(undefined, '地面不洁')).toBe('地面不洁')
  })

  it('appends a dictionary phrase while preserving editable text', () => {
    expect(appendHygieneSituationSnippet('门窗有灰尘  ', '地面不洁')).toBe('门窗有灰尘\n地面不洁')
  })
})
