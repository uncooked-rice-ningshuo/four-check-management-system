import { useInfiniteQuery } from '@tanstack/react-query'
import { Button, Select, Spin, Typography } from 'antd'
import { useEffect, useMemo, useState } from 'react'
import { courseInspectionApi, type InspectionQuery } from '../../../shared/api/courseInspectionApi'

const pageSize = 20
const maxSearchLength = 128
const loadingOptionValue = '__course_options_loading__'
type CourseScope = Pick<InspectionQuery, 'semesterId' | 'weekId' | 'inspectionTypeId' | 'departmentId' | 'recordId'>
type Props = {
  id?: string
  value?: string
  onChange?: (value?: string) => void
  scope: CourseScope
  selectedLabel?: string
  disabled?: boolean
}

function limitSearchLength(value: string) {
  return Array.from(value).slice(0, maxSearchLength).join('')
}

/** Search the shared course catalog without loading it all into the browser. */
export function CourseSelect({ id, value, onChange, scope, selectedLabel, disabled = false }: Props) {
  const [open, setOpen] = useState(false)
  const [searchInput, setSearchInput] = useState('')
  const [keyword, setKeyword] = useState('')
  const [selectedOption, setSelectedOption] = useState<{ id: string; label: string }>()
  const ready = Boolean(scope.semesterId && scope.weekId && scope.inspectionTypeId && scope.departmentId)

  useEffect(() => {
    const timer = window.setTimeout(() => setKeyword(searchInput), 250)
    return () => window.clearTimeout(timer)
  }, [searchInput])

  const query = useInfiniteQuery({
    queryKey: ['inspection-course-options', scope, keyword],
    initialPageParam: 1,
    queryFn: ({ pageParam }) => courseInspectionApi.courseOptions({ ...scope, keyword, page: pageParam, pageSize }),
    getNextPageParam: lastPage => lastPage.page * lastPage.pageSize < lastPage.total ? lastPage.page + 1 : undefined,
    enabled: open && ready && keyword === searchInput,
  })
  const loadedOptions = useMemo(() => {
    if (keyword !== searchInput) return []
    const seen = new Set<string>()
    return (query.data?.pages ?? []).flatMap(page => page.items).filter(option => {
      if (seen.has(option.id)) return false
      seen.add(option.id)
      return true
    })
  }, [keyword, query.data?.pages, searchInput])
  const options: { value: string; label: string; disabled?: boolean }[] = loadedOptions.map(option => ({ value: option.id, label: option.label }))
  if (value && !options.some(option => option.value === value)) {
    const fallback = selectedOption?.id === value ? selectedOption.label : selectedLabel
    if (fallback) options.unshift({ value, label: fallback })
  }
  if (query.isFetchingNextPage) options.push({ value: loadingOptionValue, label: '正在加载更多课程…', disabled: true })

  const loadNextPage = () => {
    if (query.hasNextPage && !query.isFetchingNextPage && !query.isFetchNextPageError) void query.fetchNextPage()
  }

  return <Select
    id={id}
    value={value}
    open={open}
    disabled={disabled}
    allowClear
    showSearch
    filterOption={false}
    searchValue={searchInput}
    options={options}
    loading={query.isFetching || keyword !== searchInput}
    listHeight={320}
    placeholder="搜索并选择课程"
    notFoundContent={query.isError
      ? <Button size="small" type="link" onClick={() => void query.refetch()}>加载失败，点击重试</Button>
      : query.isLoading || keyword !== searchInput ? <Spin size="small" /> : <Typography.Text type="secondary">没有匹配的课程</Typography.Text>}
    popupRender={menu => <>
      {menu}
      {query.isFetchNextPageError && <div style={{ padding: '4px 12px', textAlign: 'center' }}><Button size="small" type="link" onClick={() => void query.fetchNextPage()}>加载失败，点击重试</Button></div>}
    </>}
    onOpenChange={setOpen}
    onSearch={text => setSearchInput(limitSearchLength(text))}
    onClear={() => { setSelectedOption(undefined); onChange?.(undefined) }}
    onChange={(nextValue: string | undefined) => {
      const selected = loadedOptions.find(option => option.id === nextValue)
      if (selected) setSelectedOption(selected)
      else if (!nextValue) setSelectedOption(undefined)
      onChange?.(nextValue)
      setSearchInput('')
    }}
    onPopupScroll={event => {
      const target = event.target as HTMLElement
      if (target.scrollTop + target.clientHeight >= target.scrollHeight - 24) loadNextPage()
    }}
  />
}
