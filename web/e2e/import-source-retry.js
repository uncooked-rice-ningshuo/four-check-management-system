async (page) => {
 const drawer=page.getByRole('dialog',{name:'批量导入到课率',exact:true})
 const [response]=await Promise.all([page.waitForResponse(r=>r.url().endsWith('/api/v1/imports/attendance')&&r.request().method()==='POST'),drawer.getByRole('button',{name:'上传并生成预览'}).click()])
 if(response.status()!==201)throw new Error('Upload retry failed')
 const batch=await response.json()
 if(batch.status!=='PREVIEW_READY'||batch.validRows!==2)throw new Error('Invalid real preview')
 await drawer.getByText('校验通过',{exact:true}).waitFor()
 await drawer.getByRole('cell',{name:'000001',exact:true}).waitFor()
 return {cases:['解除数据库故障后原抽屉直接重试，真实预览两条记录且源文件保持可读']}
}
