import { useLocation, useNavigate } from 'react-router-dom'
import { useInspectionListScope } from '../../features/attendance/hooks/useInspectionListScope'
import { InspectionImportScopePanel } from '../../features/import-job/ui/InspectionImportScopePanel'
import { PlusOutlined, UploadOutlined } from '@ant-design/icons'
import { ProTable, type ProFormInstance, type ProColumns } from '@ant-design/pro-components'
import { Alert, Button, Empty, Space, Tag, Typography } from 'antd'
import { useRef, useState } from 'react'
import { useAuth } from '../../app/providers/authContext'
import { DepartmentSubmissionActions } from '../../features/department-submission/ui/DepartmentSubmissionActions'
import type { DormitoryHygieneRecord } from '../../entities/inspection/model/dormitoryHygieneRecord'
import { useAttendancePermissions } from '../../features/attendance/hooks/useAttendancePermissions'
import { hasNavigableInspectionAction } from '../../features/attendance/hooks/hasNavigableInspectionAction'
import { useDormitoryHygieneRecords, useInspectionContext } from '../../features/dormitory-hygiene/model/queries'
import { DormitoryHygieneCreatePanel } from '../../features/dormitory-hygiene/ui/DormitoryHygieneCreatePanel'
import { DormitoryHygieneDeleteDialog, DormitoryHygienePanel } from '../../features/dormitory-hygiene/ui/DormitoryHygienePanels'

const statuses={DRAFT:'草稿',PENDING_REVIEW:'待审核',RETURNED:'已退回',CONFIRMED:'已确认',LOCKED:'已锁定'}
const sources={MANUAL:'手工录入',EXCEL:'Excel 导入',REPORT_CORRECTION:'通报修正',MIGRATION:'历史迁移'}
const results={EXCELLENT:'优秀',QUALIFIED:'合格',UNQUALIFIED:'不合格'}
export function DormitoryHygienePage() {
 const {user}=useAuth(),permissions=useAttendancePermissions()
 const location=useLocation(),navigate=useNavigate()
 const importBatchId=(location.state as {importBatchId?:string}|null)?.importBatchId
 const [importing,setImporting]=useState(!!importBatchId)
 const setImportBatchId=(id?:string)=>navigate(location.pathname+location.search,{replace:true,state:{...location.state,importBatchId:id}})
 const form=useRef<ProFormInstance>(undefined)
 const [creating,setCreating]=useState(false)
 const [panel,setPanel]=useState<{id:string;mode:'detail'|'edit'}>()
 const [deleting,setDeleting]=useState<DormitoryHygieneRecord[]>([])
 const [selected,setSelected]=useState<React.Key[]>([])
 const semesters=useInspectionContext({purpose:'view',kind:'semesters'})
 const scope=useInspectionListScope(semesters.data?.items)
 const weeks=useInspectionContext({purpose:'view',kind:'weeks',semesterId:scope.optionSemesterId},!!scope.optionSemesterId)
 const {weekId,filters,invalidSelection}=scope.resolve(weeks.data?.items)
 const {semesterId,update:saveFilters,setDraftSemester}=scope
 const departments=useInspectionContext({purpose:'view',kind:'departments'})
 const classes=useInspectionContext({purpose:'view',kind:'classes'})
 const inspectionTypeId=semesters.data?.inspectionTypeId
 const records=useDormitoryHygieneRecords({...filters,inspectionTypeId},!!inspectionTypeId&&!scope.currentWeek.isLoading&&!scope.currentWeek.isError&&!invalidSelection)
 const canCreate=user?.isSuperAdmin|| (!!inspectionTypeId&&(user?.systemRole!=='MEMBER'||!!user.primaryDepartmentId)&&hasNavigableInspectionAction(permissions.data?.permissions,'INSPECTION_ENTER',inspectionTypeId))
 const selections=(records.data?.items??[]).filter(record=>selected.includes(record.id)&&record.canEdit&&record.status==='DRAFT')
 const canReview=!!user?.isSuperAdmin|| (!!inspectionTypeId&&hasNavigableInspectionAction(permissions.data?.permissions,'INSPECTION_REVIEW',inspectionTypeId))
 const optionQueries=[scope.currentWeek,semesters,weeks,departments,classes]
 const optionError=optionQueries.some(query=>query.isError)
 const columns:ProColumns<DormitoryHygieneRecord>[]=[
  {title:'学期',dataIndex:'semesterId',hideInTable:true,valueType:'select',fieldProps:{options:semesters.data?.items.map(item=>({value:item.id,label:item.label})),onChange:(value:string)=>{setDraftSemester(value??'');form.current?.setFieldValue('weekId',undefined)},allowClear:true}},
  {title:'周次',dataIndex:'weekId',hideInTable:true,valueType:'select',fieldProps:{options:weeks.data?.items.map(item=>({value:item.id,label:item.label})),disabled:!scope.optionSemesterId,allowClear:true}},
  {title:'部门',dataIndex:'departmentId',hideInTable:true,valueType:'select',fieldProps:{options:departments.data?.items.map(item=>({value:item.id,label:item.label})),placeholder:'全部有权部门',allowClear:true}},
  {title:'班级',dataIndex:'classId',hideInTable:true,valueType:'select',fieldProps:{options:classes.data?.items.map(item=>({value:item.id,label:item.label})),showSearch:true,optionFilterProp:'label',allowClear:true}},
  {title:'数据状态',dataIndex:'status',valueType:'select',valueEnum:statuses,width:110},
  {title:'数据来源',dataIndex:'source',valueType:'select',valueEnum:sources,width:110},
  {title:'寝室（当前名称）',dataIndex:'currentDormitoryName',search:false,width:230},
  {title:'检查日期',key:'date',search:false,width:160,render:(_,record)=>record.details.date},
  {title:'时间/轮次',key:'occurrence',search:false,width:130,render:(_,record)=>record.details.time||record.details.round||'—'},
  {title:'检查结果',key:'result',search:false,width:100,render:(_,record)=>results[record.details.result]},
  {title:'责任班级',key:'responsibleClasses',search:false,width:220,render:(_,record)=>record.responsibleClasses.map(item=>item.label).join('、')},
  {title:'不合格原因',key:'reason',search:false,width:150,render:(_,record)=>record.currentReasonLabel||record.details.reasonCode||'—'},
  {title:'操作',valueType:'option',fixed:'right',width:280,render:(_,record)=><Space>{record.importBatchId&&record.canEdit&&<Button type="link" onClick={()=>{setImportBatchId(record.importBatchId);setImporting(true)}}>导入批次</Button>}<Button type="link" onClick={()=>setPanel({id:record.id,mode:'detail'})}>详情</Button>{record.canEdit&&<><Button type="link" onClick={()=>setPanel({id:record.id,mode:'edit'})}>编辑</Button>{record.status==='DRAFT'&&<Button type="link" danger onClick={()=>setDeleting([record])}>删除</Button>}</>}</Space>},
 ]
 return <section className="attendance-page" aria-labelledby="inspection-page-title">
 <div className="attendance-page-heading"><div><Typography.Title id="inspection-page-title" level={2}>宿舍卫生检查</Typography.Title>
 <Typography.Text type="secondary">按授权范围查询宿舍卫生记录；新增时单独选择录入目标。</Typography.Text></div></div>
 {scope.currentWeek.data?.week===null&&<Alert type="info" title="当前无有效教学周，请手动选择学期和周次。"/>}
 {invalidSelection&&<Alert type="error" title="所选学期或周次已不在授权范围内，请重置筛选条件。" action={<Button onClick={()=>saveFilters({page:1,pageSize:20})}>重置筛选</Button>}/>}
 {optionError&&<Alert type="error" title="筛选选项加载失败，请重试。" action={<Button onClick={()=>optionQueries.forEach(query=>{if(query.isError)void query.refetch()})}>重试</Button>}/>}
 <ProTable<DormitoryHygieneRecord> className="attendance-records-table" cardBordered rowKey="id" columns={columns} dataSource={records.isError?[]:records.data?.items??[]} loading={records.isLoading||semesters.isLoading||scope.currentWeek.isLoading}
 headerTitle={<Space size={10}><span>宿舍卫生记录</span><Tag color="blue">共 {records.data?.total??0} 条</Tag></Space>}
 key={`${semesterId??''}-${weekId??''}`} formRef={form} form={{initialValues:filters}} search={{labelWidth:'auto',defaultCollapsed:false}} options={{reload:()=>void records.refetch(),density:true,setting:true}} onSubmit={values=>{saveFilters({...values,page:1,pageSize:filters.pageSize});setSelected([])}} onReset={()=>{saveFilters({page:1,pageSize:20});setSelected([])}}
 scroll={{x:1500}} pagination={{current:Number(filters.page),pageSize:Number(filters.pageSize),total:records.data?.total??0,showSizeChanger:true,showQuickJumper:true,showTotal:total=>`共 ${total} 条`,onChange:(page,pageSize)=>{saveFilters({...filters,page,pageSize});setSelected([])}}}
 rowSelection={{selectedRowKeys:selected,onChange:setSelected,getCheckboxProps:record=>({disabled:!record.canEdit||record.status!=='DRAFT'})}}
 locale={{emptyText:invalidSelection?'所选学期或周次不可用':records.isError?<Alert type="error" title="记录加载失败，请重试。" action={<Button onClick={()=>void records.refetch()}>重试</Button>}/>:optionError?'请先恢复筛选选项加载':<Empty description="当前范围暂无宿舍卫生记录"/>}}
 toolBarRender={()=>[<Space key="actions" wrap style={{minWidth:0,maxWidth:'100%'}}>{canCreate?<><Button key="create" icon={<PlusOutlined aria-hidden/>} onClick={()=>setCreating(true)}>新增记录</Button><Button type="primary" icon={<UploadOutlined aria-hidden/>} onClick={()=>setImporting(true)}>批量导入宿舍卫生</Button></>:null}<DepartmentSubmissionActions inspectionTypeId={inspectionTypeId??''} listScope={filters} selected={selections} canReview={canReview} scopeLabels={{semesters:semesters.data?.items,weeks:weeks.data?.items,departments:departments.data?.items,inspectionTypeName:'宿舍卫生'}} onChanged={()=>{setSelected([]);void records.refetch()}} onOpenRecord={(id,status)=>setPanel({id,mode:status==='RETURNED'?'edit':'detail'})}/>{selections.length>0&&<Button key="delete" danger onClick={()=>setDeleting(selections)}>批量删除（{selections.length}）</Button>}</Space>]}/>
 {importing&&inspectionTypeId&&<InspectionImportScopePanel memberDepartmentId={user?.systemRole==='MEMBER'?user.primaryDepartmentId:undefined} typeCode="DORMITORY_HYGIENE" inspectionTypeId={inspectionTypeId} batchId={importBatchId} onBatchIdChange={setImportBatchId} onClose={()=>setImporting(false)}/>}
 {creating&&inspectionTypeId&&<DormitoryHygieneCreatePanel inspectionTypeId={inspectionTypeId} memberDepartmentId={user?.systemRole==='MEMBER'?user.primaryDepartmentId:undefined} onClose={()=>setCreating(false)}/>}
 {panel&&<DormitoryHygienePanel key={panel.id+panel.mode} {...panel} memberDepartmentId={user?.systemRole==='MEMBER'?user.primaryDepartmentId:undefined} onClose={()=>setPanel(undefined)}/>}
 {deleting.length>0&&<DormitoryHygieneDeleteDialog records={deleting} onClose={()=>{setDeleting([]);setSelected([])}}/>}
 </section>
}
