import { expect, it } from 'vitest'
import { naturalClassOrder } from './classOrder'

it('按阿拉伯数字、全角数字与中文班号混排且保留原数组', () => {
  const names = ['计科十一班', '计科１０班', '计科二班', '计科1班', '计科一百零二班']
  expect(naturalClassOrder(names, names.map(name => ({ id: name, name })))).toEqual([names[3], names[2], names[1], names[0], names[4]])
  expect(names[0]).toBe('计科十一班')
})
