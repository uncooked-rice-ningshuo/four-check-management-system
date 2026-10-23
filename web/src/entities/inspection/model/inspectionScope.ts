// API selection value; the server stores college ownership as a null department.
export const COLLEGE_DEPARTMENT_OPTION_ID = '00000000-0000-0000-0000-000000000000'

export const departmentOptionId = (departmentId: string | undefined) => departmentId || COLLEGE_DEPARTMENT_OPTION_ID
