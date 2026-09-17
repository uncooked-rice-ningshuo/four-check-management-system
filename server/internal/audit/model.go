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
	ActionFileIntegrityAnomaly    Action = "FILE_INTEGRITY_ANOMALY"
	ActionInspectionRecordUpdated Action = "INSPECTION_RECORD_UPDATED"
	ActionInspectionRecordDeleted Action = "INSPECTION_RECORD_DELETED"
	ActionInspectionRecordCreated Action = "INSPECTION_RECORD_CREATED"
	ActionImportBatchConfirmed    Action = "IMPORT_BATCH_CONFIRMED"
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
