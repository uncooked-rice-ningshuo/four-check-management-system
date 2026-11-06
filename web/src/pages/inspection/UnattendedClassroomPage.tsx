import { useLocation, useNavigate } from 'react-router-dom'
import { useInspectionListScope } from '../../features/attendance/hooks/useInspectionListScope'
import { createInspectionFilterColumns, createInspectionProvenanceColumns, createInspectionSortColumn } from '../../features/inspection-list/model/inspectionColumns'
import { UnattendedImportPanel } from '../../features/import-job/ui/UnattendedImportPanel'
import { PlusOutlined, UploadOutlined } from '@ant-design/icons'
import { ProTable, type ProFormInstance, type ProColumns } from '@ant-design/pro-components'
import { Alert, Button, Empty, Space, Tag, Typography } from 'antd'
import { useEffect, useRef, useState } from 'react'
import { useAuth } from '../../app/providers/authContext'
import { DepartmentSubmissionActions } from '../../features/department-submission/ui/DepartmentSubmissionActions'
import type { CourseRecord } from '../../entities/inspection/model/courseRecord'
import { attributionLabel } from '../../entities/inspection/model/inspectionScope'
import { useAttendancePermissions } from '../../features/attendance/hooks/useAttendancePermissions'
import { hasNavigableInspectionAction } from '../../features/attendance/hooks/hasNavigableInspectionAction'
import { useCourseRecords, useInspectionContext } from '../../features/course-inspection/model/queries'
import { CourseCreatePanel } from '../../features/course-inspection/ui/CourseCreatePanel'
import { CourseDeleteDialog, CourseRecordPanel } from '../../features/course-inspection/ui/CourseRecordPanels'

export function UnattendedClassroomPage() {
 const {user}=useAuth(),permissions=useAttendancePermissions()
 const location=useLocation(),navigate=useNavigate()
 const importBatchId=(location.state as {importBatchId?:string}|null)?.importBatchId
 const [importing,setImporting]=useState(!!importBatchId)
 const setImportBatchId=(id?:string)=>navigate(location.pathname+location.search,{replace:true,state:{...location.state,importBatchId:id}})
 const form=useRef<ProFormInstance>(undefined)
 const [selectedGrade,setSelectedGrade]=useState<string>()
 const [creating,setCreating]=useState(false)
 const [panel,setPanel]=useState<{id:string;mode:'detail'|'edit'}>()
 const [deleting,setDeleting]=useState<CourseRecord[]>([])
 const [selected,setSelected]=useState<React.Key[]>([])
 const semesters=useInspectionContext({purpose:'view',kind:'semesters'})
 const scope=useInspectionListScope(semesters.data?.items)
 const weeks=useInspectionContext({purpose:'view',kind:'weeks',semesterId:scope.optionSemesterId},!!scope.optionSemesterId)
 const {weekId,filters,invalidSelection}=scope.resolve(weeks.data?.items)
 const {semesterId,update:saveFilters,setDraftSemester}=scope
 const previousFilterSemester=useRef(scope.optionSemesterId)
 useEffect(()=>{
  if(previousFilterSemester.current!==scope.optionSemesterId){previousFilterSemester.current=scope.optionSemesterId;form.current?.setFieldValue('weekId',undefined)}
 },[scope.optionSemesterId])
 const previousFilterGrade=useRef(selectedGrade)
 useEffect(()=>{
  if(previousFilterGrade.current!==selectedGrade){previousFilterGrade.current=selectedGrade;form.current?.setFieldValue('classId',undefined)}
 },[selectedGrade])
 const departments=useInspectionContext({purpose:'view',kind:'departments'})
 const classes=useInspectionContext({purpose:'view',kind:'classes'})
 const grades=useInspectionContext({purpose:'view',kind:'grades'})
 const inspectionTypeId=semesters.data?.inspectionTypeId
 const records=useCourseRecords({...filters,inspectionTypeId},!!inspectionTypeId&&!!semesterId&&!!weekId&&!scope.currentWeek.isLoading&&!scope.currentWeek.isError&&!invalidSelection)
 const canCreate=user?.isSuperAdmin|| (!!inspectionTypeId&&(user?.systemRole!=='MEMBER'||!!user.primaryDepartmentId)&&hasNavigableInspectionAction(permissions.data?.permissions,'INSPECTION_ENTER',inspectionTypeId))
 const selections=(records.data?.items??[]).filter(record=>selected.includes(record.id)&&record.canDelete)
 const canReview=!!user?.isSuperAdmin|| (!!inspectionTypeId&&hasNavigableInspectionAction(permissions.data?.permissions,'INSPECTION_REVIEW',inspectionTypeId))
 const optionQueries=[scope.currentWeek,semesters,weeks,departments,classes,grades]
 const optionError=optionQueries.some(query=>query.isError)
 const gradeFilter=selectedGrade===undefined?String(filters.grade??''):selectedGrade
 const columns:ProColumns<CourseRecord>[]=[
  ...createInspectionFilterColumns<CourseRecord>({
   semesters:(semesters.data?.items??[]).map(item=>({id:item.id,label:item.label})),
   weeks:(weeks.data?.items??[]).map(item=>({id:item.id,label:item.label})),
   departments:(departments.data?.items??[]).map(item=>({id:item.id,label:item.label})),
   grades:(grades.data?.items??[]).map(item=>({id:item.id,label:item.label})),
   classes:(classes.data?.items??[]).map(item=>({id:item.id,label:item.label,grade:item.grade})),
   selectedSemesterId:scope.optionSemesterId,
   onSemesterChange:value=>setDraftSemester(value??''),
   selectedGrade:gradeFilter,
   onGradeChange:value=>setSelectedGrade(value??''),
  }),
  createInspectionSortColumn<CourseRecord>(),
  {title:'学号',dataIndex:'studentNumber',search:false,width:140},
  {title:'姓名',dataIndex:'studentName',search:false,width:120},
  {title:'年级',dataIndex:'grade',search:false,width:90},
  {title:'班级',dataIndex:'className',search:false,width:150},
  {title:'课程（当前名称）',dataIndex:'currentCourseName',search:false,width:180},
  {title:'教室（当前名称）',dataIndex:'currentClassroomName',search:false,width:180},
  {title:'检查日期',key:'date',search:false,width:140,render:(_,record)=>record.details.date},
  
  
  ...createInspectionProvenanceColumns<CourseRecord>(),
  {title:'操作',valueType:'option',fixed:'right',width:280,render:(_,record)=><Space>{record.importBatchId&&record.canEdit&&<Button type="link" onClick={()=>{setImportBatchId(record.importBatchId);setImporting(true)}}>导入批次</Button>}<Button type="link" onClick={()=>setPanel({id:record.id,mode:'detail'})}>详情</Button>{record.canEdit&&<Button type="link" onClick={()=>setPanel({id:record.id,mode:'edit'})}>编辑</Button>}{record.canDelete&&<Button type="link" danger onClick={()=>setDeleting([record])}>删除</Button>}</Space>},
 ]
 const attributionColumn:ProColumns<CourseRecord>|undefined=user?.isSuperAdmin?{title:'记录归属',key:'attribution',search:false,width:160,render:(_,record)=>attributionLabel(record.attributionMode,record.departmentId,departments.data?.items.find(item=>item.id===record.departmentId)?.label)}:undefined
const displayColumns=attributionColumn?[...columns.slice(0,-1),attributionColumn,...columns.slice(-1)]:columns
 return <section className="attendance-page" aria-labelledby="inspection-page-title">
 <div className="attendance-page-heading"><div><Typography.Title id="inspection-page-title" level={2}>无机课堂检查</Typography.Title>
 <Typography.Text type="secondary">按授权范围查询课程缺勤记录；新增时单独选择录入目标。</Typography.Text></div></div>
 {invalidSelection&&<Alert type="error" title="所选学期或周次已不在授权范围内，请重置筛选条件。" action={<Button onClick={()=>saveFilters({page:1,pageSize:20})}>重置筛选</Button>}/>}
 {optionError&&<Alert type="error" title="筛选选项加载失败，请重试。" action={<Button onClick={()=>optionQueries.forEach(query=>{if(query.isError)void query.refetch()})}>重试</Button>}/>}
 <ProTable<CourseRecord> className="attendance-records-table" cardBordered rowKey="id" columns={displayColumns} dataSource={records.isError?[]:records.data?.items??[]} loading={records.isLoading||semesters.isLoading||scope.currentWeek.isLoading}
 headerTitle={<Space size={10}><span>无机课堂记录</span><Tag color="blue">共 {records.data?.total??0} 条</Tag></Space>}
 key={`${semesterId??''}-${weekId??''}`} formRef={form} form={{initialValues:filters}} search={{labelWidth:'auto',defaultCollapsed:false}} options={{reload:()=>void records.refetch(),density:true,setting:true}} onSubmit={values=>{saveFilters({...values,page:1,pageSize:filters.pageSize});setSelected([])}} onReset={()=>{setSelectedGrade('');saveFilters({page:1,pageSize:20});setSelected([])}}
 scroll={{x:1850}} pagination={{current:Number(filters.page),pageSize:Number(filters.pageSize),total:records.data?.total??0,showSizeChanger:true,showQuickJumper:true,showTotal:total=>`共 ${total} 条`,onChange:(page,pageSize)=>{saveFilters({...filters,page,pageSize});setSelected([])}}}
 rowSelection={{selectedRowKeys:selected,onChange:setSelected,getCheckboxProps:record=>({disabled:!record.canDelete})}}
 locale={{emptyText:invalidSelection?'所选学期或周次不可用':records.isError?<Alert type="error" title="记录加载失败，请重试。" action={<Button onClick={()=>void records.refetch()}>重试</Button>}/>:optionError?'请先恢复筛选选项加载':<Empty description={scope.currentWeek.data?.week===null?'当前没有有效教学周，请选择学期和周次':'当前范围暂无无机课堂记录'}/>}}
 toolBarRender={()=>[<Space key="actions" wrap style={{minWidth:0,maxWidth:'100%'}}>{canCreate?<><Button key="create" icon={<PlusOutlined aria-hidden/>} onClick={()=>setCreating(true)}>新增记录</Button><Button type="primary" icon={<UploadOutlined aria-hidden/>} onClick={()=>setImporting(true)}>批量导入</Button></>:null}<DepartmentSubmissionActions typeCode="UNATTENDED_CLASSROOM" inspectionTypeId={inspectionTypeId??''} listScope={filters} selected={selections} canReview={canReview} onChanged={()=>{setSelected([]);void records.refetch()}} onOpenRecord={(id,status)=>setPanel({id,mode:status==='RETURNED'?'edit':'detail'})}/>{selections.length>0&&<Button key="delete" danger onClick={()=>setDeleting(selections)}>批量删除（{selections.length}）</Button>}</Space>]}/>
 {importing&&inspectionTypeId&&<UnattendedImportPanel inspectionTypeId={inspectionTypeId} memberDepartmentId={user?.systemRole==='MEMBER'?user.primaryDepartmentId:undefined} isSuperAdmin={!!user?.isSuperAdmin} batchId={importBatchId} onBatchIdChange={setImportBatchId} onClose={()=>setImporting(false)}/>}
 {creating&&inspectionTypeId&&<CourseCreatePanel inspectionTypeId={inspectionTypeId} memberDepartmentId={user?.systemRole==='MEMBER'?user.primaryDepartmentId:undefined} isSuperAdmin={!!user?.isSuperAdmin} onClose={()=>setCreating(false)}/>}
 {panel&&<CourseRecordPanel key={panel.id+panel.mode} {...panel} memberDepartmentId={user?.systemRole==='MEMBER'?user.primaryDepartmentId:undefined} isSuperAdmin={!!user?.isSuperAdmin} onClose={()=>setPanel(undefined)}/>}
 {deleting.length>0&&<CourseDeleteDialog records={deleting} onClose={()=>{setDeleting([]);setSelected([])}}/>}
 </section>
}
