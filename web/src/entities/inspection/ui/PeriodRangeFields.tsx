import { Col, Form, Select } from 'antd'
import { COURSE_PERIOD_NUMBERS } from '../model/coursePeriod'

type Props = { startName: string; endName: string }

const required = [{ required: true, message: '请选择节次' }]
const periodOptions = COURSE_PERIOD_NUMBERS.map(value => ({ value, label: `第 ${value} 节` }))

export function PeriodRangeFields({ startName, endName }: Props) {
  const form = Form.useFormInstance()

  return <>
    <Col xs={24} sm={12}>
      <Form.Item name={startName} label="开始节次" rules={required}>
        <Select placeholder="选择开始节次" options={periodOptions} onChange={(start: number) => {
          const end = form.getFieldValue(endName)
          if (typeof end === 'number' && end < start) form.setFields([{ name: endName, value: undefined, errors: [] }])
        }} />
      </Form.Item>
    </Col>
    <Col xs={24} sm={12}>
      <Form.Item noStyle shouldUpdate={(previous, current) => previous[startName] !== current[startName]}>
        {() => {
          const start = form.getFieldValue(startName) as number | undefined
          return <Form.Item name={endName} label="结束节次" dependencies={[startName]} rules={[
            ...required,
            { validator: (_: unknown, value?: number) => value === undefined || start === undefined || value >= start ? Promise.resolve() : Promise.reject(new Error('结束节次不能早于开始节次')) },
          ]}>
            <Select disabled={start === undefined} placeholder="选择结束节次" options={periodOptions.filter(option => start === undefined || option.value >= start)} />
          </Form.Item>
        }}
      </Form.Item>
    </Col>
  </>
}
