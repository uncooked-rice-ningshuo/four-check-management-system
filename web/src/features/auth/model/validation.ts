export type LoginValues = { studentNo: string; password: string }
export type RegistrationValues = LoginValues & { name: string; confirmPassword: string }

export function validateLogin(values: LoginValues) {
  const errors: Partial<Record<keyof LoginValues, string>> = {}
  if (!values.studentNo.trim()) errors.studentNo = '请输入学号'
  if (!values.password) errors.password = '请输入密码'
  return errors
}

export function validateRegistration(values: RegistrationValues) {
  const errors: Partial<Record<keyof RegistrationValues, string>> = {}
  if (!values.studentNo.trim()) errors.studentNo = '请输入学号'
  if (!values.name.trim()) errors.name = '请输入姓名'
  if (values.password.length < 8) errors.password = '密码至少 8 位'
  if (values.confirmPassword !== values.password) errors.confirmPassword = '两次输入的密码不一致'
  return errors
}
