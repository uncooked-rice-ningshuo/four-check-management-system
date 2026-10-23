async(page)=>{
 const fixture=__FIXTURE__,base=page.url().split('/').slice(0,3).join('/'),contexts=[],cases=[],browserErrors=[]
 const check=(ok,message)=>{if(!ok)throw new Error(message)}
 const request=async(c,token,path,method='GET',data,status=200)=>{
  const r=await c.request.fetch(base+'/api/v1'+path,{method,data,headers:token?{Authorization:'Bearer '+token}:{}})
  check(r.status()===status,`${method} ${path.split('?')[0]} expected ${status}, got ${r.status()}`);return r
 }
 const newContext=async()=>{const c=await page.context().browser().newContext();contexts.push(c);return c}
 try{
  const admin=await newContext(),token=(await(await request(admin,'','/auth/login','POST',{loginName:'browser-admin',password:fixture.authPassword})).json()).accessToken
  const catalogs=await(await request(admin,'','/public/registration-options')).json(),dorm=catalogs.departments.find(d=>d.code==='DORMITORY'),study=catalogs.departments.find(d=>d.code==='STUDY'),member=catalogs.organizationRoles.find(r=>r.code==='MEMBER')
  const calendar=await(await request(admin,token,'/admin/academic/calendar')).json()
  const year='b0000000-0000-0000-0000-000000000001'
  const existingOther=calendar.academicYears.find(y=>y.id!==year)
  const other=existingOther?.id??(await(await request(admin,token,'/admin/academic/years','POST',{startDate:'2050-09-01T00:00:00+08:00',endDate:'2051-08-31T00:00:00+08:00'},201)).json()).id
  const type=(await(await request(admin,token,'/inspections/attendance/filter-options')).json()).inspectionType.id
  for(const scenario of ['current','other','deny-current','deny-other','deny-class']){
   const c=await newContext(),name='filter-scope-'+scenario
   const account=await(await request(c,'','/auth/register','POST',{studentNo:name,name,password:fixture.authPassword,requestedDepartmentId:dorm.id,requestedOrgRoleId:member.id},201)).json()
   await request(admin,token,'/admin/registration-requests/'+account.id+'/review','POST',{decision:'APPROVE',reason:'isolated option scope'})
   await request(admin,token,'/admin/users/'+account.id+'/permission-rules','POST',{action:'INSPECTION_VIEW',effect:'ALLOW',scope:{academicYearId:scenario==='other'?other:year,inspectionTypeId:type,departmentId:study.id},enabled:true,note:'isolated option scope'},201)
   if(scenario.startsWith('deny-'))await request(admin,token,'/admin/users/'+account.id+'/permission-rules','POST',{action:'INSPECTION_VIEW',effect:'DENY',scope:scenario==='deny-class'?{classId:fixture.class}:{academicYearId:scenario==='deny-current'?year:other},enabled:true,note:'isolated option denial'},201)
   const access=(await(await request(c,'','/auth/login','POST',{loginName:name,password:fixture.authPassword})).json()).accessToken
   const options=await(await request(c,access,'/inspections/attendance/filter-options')).json()
   const expectedYear=scenario==='other'?other:year
   check(options.semesters.every(s=>s.academicYearId===expectedYear),'Semester escaped academic-year grant')
   check(options.weeks.every(w=>options.semesters.some(s=>s.id===w.semesterId)),'Week escaped authorized semesters')
   if(scenario==='deny-current'||scenario==='deny-class')check(['semesters','weeks','departments','responsibleDepartments','classes','grades'].every(k=>options[k].length===0),'DENY leaked option metadata')
   if(scenario==='current'||scenario==='deny-other')check(options.classes.some(c=>c.id===fixture.class&&c.grade==='2026级')&&options.semesters.length>0,'Valid grant or current class grade lost')
   const p=await c.newPage();p.on('pageerror',e=>browserErrors.push(e.message))
   await p.goto(base+'/inspections/attendance')
   if(scenario==='deny-current')await p.getByText('无权访问该业务',{exact:true}).waitFor()
   else await p.getByRole('heading',{name:'到课率检查',exact:true}).waitFor().catch(()=>{throw new Error(scenario+': expected attendance page, observed '+p.url())})
   check(await p.getByText('无法加载到课率筛选项，请稍后重试',{exact:true}).count()===0,'Valid options failed UI loading')
   cases.push(scenario+'：真实账号学年/单独拒绝过滤、班级年级元数据与到课率页面加载')
  }
  const noAuth=await newContext();await request(noAuth,'','/inspections/attendance/filter-options','GET',undefined,401)
  check(browserErrors.length===0,'Browser runtime error')
  return {cases,browserErrors,browserVersion:page.context().browser().version()}
 }finally{for(const c of contexts.reverse())await c.close()}
}
