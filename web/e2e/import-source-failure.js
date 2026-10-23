async (page) => {
 const fixture=__FIXTURE__
 const base=page.url().split('/').slice(0,3).join('/')
 await page.goto(base+'/inspections/attendance')
 await page.getByRole('button',{name:'批量导入到课率',exact:true}).click()
 const drawer=page.getByRole('dialog',{name:'批量导入到课率',exact:true})
 await drawer.getByRole('combobox',{name:'导入目标部门'}).click()
 await page.locator('.ant-select-dropdown:visible').getByText('学习部',{exact:true}).click()
 await drawer.locator('input[type=file]').setInputFiles(fixture.validFile)
 const [response]=await Promise.all([page.waitForResponse(r=>r.url().endsWith('/api/v1/imports/attendance')&&r.request().method()==='POST'),drawer.getByRole('button',{name:'上传并生成预览'}).click()])
 if(response.status()!==500||(await response.json()).code!=='IMPORT_SERVICE_UNAVAILABLE')throw new Error('Missing real batch failure contract')
 await drawer.getByText('操作失败，请稍后重试',{exact:true}).waitFor()
 if(!await drawer.getByRole('button',{name:'上传并生成预览'}).isEnabled())throw new Error('Upload retry unavailable')
 return {cases:['真实数据库拒绝创建批次：500稳定错误码、保留上传文件并可重试'],faultInjection:['一次性 PostgreSQL import_batches INSERT 触发器拒绝']}
}
