package audit

import (
	"context"
	"errors"
	"time"
)

// Action 是写入审计日志的稳定动作标识，供查询和合规核查使用。
type Action string

const (
	ActionRegistrationReviewed    Action = "REGISTRATION_REVIEWED"
	ActionAccountCreated          Action = "ACCOUNT_CREATED"
	ActionAccountUpdated          Action = "ACCOUNT_UPDATED"
	ActionAccountStatusChanged    Action = "ACCOUNT_STATUS_CHANGED"
	ActionAccountPasswordReset    Action = "ACCOUNT_PASSWORD_RESET"
	ActionOrganizationUpdated     Action = "ORGANIZATION_UPDATED"
	ActionPermissionRuleChanged   Action = "PERMISSION_RULE_CHANGED"
	ActionAcademicCalendarChanged Action = "ACADEMIC_CALENDAR_CHANGED"
	ActionMasterDataChanged       Action = "MASTER_DATA_CHANGED"
	ActionDictionaryChanged       Action = "DICTIONARY_CHANGED"
	ActionFileUploadSucceeded     Action = "FILE_UPLOAD_SUCCEEDED"
	ActionFileUploadFailed        Action = "FILE_UPLOAD_FAILED"
	ActionFileRead                Action = "FILE_READ"
	ActionFileObjectRemoved       Action = "FILE_OBJECT_REMOVED"
	ActionFileCleanupFailed       Action = "FILE_CLEANUP_FAILED"
	ActionFileRecoveryDeferred    Action = "FILE_RECOVERY_DEFERRED"
	ActionFileIntegrityAnomaly    Action = "FILE_INTEGRITY_ANOMALY"
	ActionInspectionRecordUpdated Action = "INSPECTION_RECORD_UPDATED"
	ActionInspectionRecordDeleted Action = "INSPECTION_RECORD_DELETED"
	ActionInspectionRecordCreated Action = "INSPECTION_RECORD_CREATED"
	ActionImportBatchCancelled    Action = "IMPORT_CANCELLED"
	ActionImportBatchPreviewed    Action = "IMPORT_BATCH_PREVIEWED"
	ActionImportBatchConfirmed    Action = "IMPORT_BATCH_CONFIRMED"
	ActionImportBatchRolledBack   Action = "IMPORT_BATCH_ROLLED_BACK"
	ActionDepartmentSubmitted     Action = "DEPARTMENT_SUBMITTED"
	ActionDepartmentReturned      Action = "DEPARTMENT_RETURNED"
	ActionDepartmentConfirmed     Action = "DEPARTMENT_CONFIRMED"
	ActionDepartmentResubmitted   Action = "DEPARTMENT_RESUBMITTED"
	ActionReportObligationChanged Action = "REPORT_OBLIGATION_CHANGED"
	ActionReportDraftGenerated    Action = "REPORT_DRAFT_GENERATED"
	ActionReportWorkflowChanged   Action = "REPORT_WORKFLOW_CHANGED"
	ActionReportCorrectionAdded   Action = "REPORT_CORRECTION_ADDED"
	ActionReportAdjustmentAdded   Action = "REPORT_ADJUSTMENT_ADDED"
)

var (
	ErrActionRequired       = errors.New("audit action is required")
	ErrResourceTypeRequired = errors.New("audit resource type is required")
	ErrResourceIDRequired   = errors.New("audit resource id is required")
)

// Metadata 保存请求级审计上下文，不包含请求体、Cookie 或认证令牌。
type Metadata struct {
	ActorID   string
	RequestID string
	IP        string
	UserAgent string
	Reason    string
}

// Entry 描述一次可追溯的资源变化；Before 和 After 必须可编码为 JSON。
type Entry struct {
	ID           string
	Action       Action
	ResourceType string
	ResourceID   string
	Before       any
	After        any
	CreatedAt    time.Time
}

// Mutation 把请求元数据与应用服务选定的审计动作一起传给持久化层。
type Mutation struct {
	Metadata Metadata
	Entry    Entry
}

// Recorder 只提供追加能力，禁止通过业务层修改或删除历史审计记录。
type Recorder interface {
	Record(ctx context.Context, metadata Metadata, entry Entry) error
}
