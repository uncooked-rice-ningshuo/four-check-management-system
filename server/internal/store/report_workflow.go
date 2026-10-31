package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/reporting"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type reportSourceRecord struct {
	ID             string
	RowVersion     int
	SubmissionID   string
	RevisionNo     int
	LockedReportID string
}

type reportZeroReceipt struct {
	ID               string
	RevisionNo       int
	DepartmentID     string
	InspectionTypeID string
}

// CreateReportDraft stores a count-and-details version and locks the exact
// confirmed records used by it in the same transaction.
func (p *Postgres) CreateReportDraft(ctx context.Context, command reporting.CreateReportDraftCommand) (reporting.ReportDraft, error) {
	if _, err := uuid.Parse(command.ReportID); err != nil {
		return reporting.ReportDraft{}, reporting.ErrInvalidReportRequest
	}
	if _, err := uuid.Parse(command.VersionID); err != nil {
		return reporting.ReportDraft{}, reporting.ErrInvalidReportRequest
	}
	if len(command.Cells) == 0 || command.ActorID == "" {
		return reporting.ReportDraft{}, reporting.ErrInvalidReportRequest
	}
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return reporting.ReportDraft{}, err
	}
	defer tx.Rollback(ctx)

	var reportID, status string
	var currentVersion, rowVersion int
	var currentSourceFingerprint string
	err = tx.QueryRow(ctx, `SELECT wr.id::text,wr.status,wr.current_version,wr.row_version,COALESCE(version.source_fingerprint,'')
FROM weekly_reports wr LEFT JOIN weekly_report_versions version ON version.report_id=wr.id AND version.version_no=wr.current_version
	WHERE wr.semester_id=$1::uuid AND wr.week_id=$2::uuid FOR UPDATE OF wr`, command.Content.Period.SemesterID, command.Content.Period.WeekID).
		Scan(&reportID, &status, &currentVersion, &rowVersion, &currentSourceFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		if command.ExpectedRowVersion != 0 {
			return reporting.ReportDraft{}, reporting.ErrReportVersionConflict
		}
		_, err = tx.Exec(ctx, `INSERT INTO weekly_reports(id,semester_id,week_id,status,current_version,row_version,created_by,updated_by)
 VALUES($1::uuid,$2::uuid,$3::uuid,'DRAFT',0,1,$4::uuid,$4::uuid)
 ON CONFLICT (semester_id,week_id) DO NOTHING`, command.ReportID, command.Content.Period.SemesterID,
			command.Content.Period.WeekID, command.ActorID)
		if err != nil {
			return reporting.ReportDraft{}, err
		}
		err = tx.QueryRow(ctx, `SELECT wr.id::text,wr.status,wr.current_version,wr.row_version,COALESCE(version.source_fingerprint,'')
FROM weekly_reports wr LEFT JOIN weekly_report_versions version ON version.report_id=wr.id AND version.version_no=wr.current_version
		WHERE wr.semester_id=$1::uuid AND wr.week_id=$2::uuid FOR UPDATE OF wr`, command.Content.Period.SemesterID, command.Content.Period.WeekID).
			Scan(&reportID, &status, &currentVersion, &rowVersion, &currentSourceFingerprint)
	}
	if err != nil {
		return reporting.ReportDraft{}, err
	}
	if (currentVersion == 0 && command.ExpectedRowVersion != 0) ||
		(currentVersion > 0 && command.ExpectedRowVersion != rowVersion) {
		return reporting.ReportDraft{}, reporting.ErrReportVersionConflict
	}
	if currentVersion > 0 && status != reporting.ReportStatusDraft && status != reporting.ReportStatusReturned &&
		status != reporting.ReportStatusFinalized && status != reporting.ReportStatusPublished && status != reporting.ReportStatusArchived {
		return reporting.ReportDraft{}, reporting.ErrInvalidReportTransition
	}
	versionNo := currentVersion + 1
	contentJSON, err := json.Marshal(command.Content)
	if err != nil {
		return reporting.ReportDraft{}, err
	}
	rows := make([]reportSourceRecord, 0)
	recordSnapshots := make(map[string]reporting.ReportRecordSnapshot, len(command.Records))
	for _, snapshot := range command.Records {
		if snapshot.Record.ID == "" || snapshot.RowVersion < 1 {
			return reporting.ReportDraft{}, reporting.ErrInvalidReportRequest
		}
		if _, duplicate := recordSnapshots[snapshot.Record.ID]; duplicate {
			return reporting.ReportDraft{}, reporting.ErrInvalidReportRequest
		}
		recordSnapshots[snapshot.Record.ID] = snapshot
	}
	query, args := reportSourceQuery(command.Cells, command.Content.Period.SemesterID, command.Content.Period.WeekID)
	sourceRows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return reporting.ReportDraft{}, err
	}
	for sourceRows.Next() {
		var item reportSourceRecord
		if err := sourceRows.Scan(&item.ID, &item.RowVersion, &item.SubmissionID, &item.RevisionNo, &item.LockedReportID); err != nil {
			sourceRows.Close()
			return reporting.ReportDraft{}, err
		}
		rows = append(rows, item)
	}
	if err := sourceRows.Err(); err != nil {
		sourceRows.Close()
		return reporting.ReportDraft{}, err
	}
	sourceRows.Close()
	if len(rows) != len(recordSnapshots) || len(rows) != command.Content.ConfirmedRecordCount {
		return reporting.ReportDraft{}, reporting.ErrReportVersionConflict
	}

	zeroReceipts, err := queryReportZeroReceipts(ctx, tx, command.Cells, command.Content.Period.SemesterID, command.Content.Period.WeekID)
	if err != nil {
		return reporting.ReportDraft{}, err
	}
	zeroReceiptCounts := make(map[string]int, len(command.Content.Rows))
	fingerprintParts := make([]string, 0, len(rows)+len(zeroReceipts))
	for _, row := range rows {
		snapshot, exists := recordSnapshots[row.ID]
		if !exists || snapshot.RowVersion != row.RowVersion || snapshot.Record.RowVersion != row.RowVersion {
			return reporting.ReportDraft{}, reporting.ErrReportVersionConflict
		}
		if row.LockedReportID != "" && row.LockedReportID != reportID {
			return reporting.ReportDraft{}, reporting.ErrReportVersionConflict
		}
		fingerprintParts = append(fingerprintParts, reportRecordFingerprintPart(row))
	}
	for _, receipt := range zeroReceipts {
		key := receipt.DepartmentID + "\x00" + receipt.InspectionTypeID
		zeroReceiptCounts[key]++
		fingerprintParts = append(fingerprintParts, reportZeroReceiptFingerprintPart(receipt))
	}
	for _, row := range command.Content.Rows {
		if zeroReceiptCounts[row.DepartmentID+"\x00"+row.InspectionTypeID] != row.ZeroReceiptCount {
			return reporting.ReportDraft{}, reporting.ErrReportVersionConflict
		}
	}
	sort.Strings(fingerprintParts)
	fingerprint := sha256.Sum256([]byte(strings.Join(fingerprintParts, "\n")))
	fingerprintHex := hex.EncodeToString(fingerprint[:])
	if currentVersion > 0 && (status == reporting.ReportStatusFinalized || status == reporting.ReportStatusPublished || status == reporting.ReportStatusArchived) &&
		currentSourceFingerprint == fingerprintHex {
		return reporting.ReportDraft{}, reporting.ErrInvalidReportTransition
	}
	createdAt := time.Now().UTC()
	_, err = tx.Exec(ctx, `INSERT INTO weekly_report_versions(
 id,report_id,version_no,content_json,source_fingerprint,forced_generation_reason,scope_limited,created_by,created_at)
 VALUES($1::uuid,$2::uuid,$3,$4::jsonb,$5,$6,$7,$8::uuid,$9)`, command.VersionID, reportID,
		versionNo, contentJSON, fingerprintHex, command.ForceReason, command.Content.ScopeLimited, command.ActorID, createdAt)
	if err != nil {
		return reporting.ReportDraft{}, err
	}
	for _, receipt := range zeroReceipts {
		if _, err := tx.Exec(ctx, `INSERT INTO weekly_report_version_zero_receipts(report_version_id,submission_id,source_revision_no)
VALUES($1::uuid,$2::uuid,$3)`, command.VersionID, receipt.ID, receipt.RevisionNo); err != nil {
			return reporting.ReportDraft{}, err
		}
	}
	for _, row := range rows {
		snapshot := recordSnapshots[row.ID]
		tag, err := tx.Exec(ctx, `UPDATE inspection_records SET locked_by_report_id=$2::uuid
 WHERE id=$1::uuid AND row_version=$3 AND status='CONFIRMED' AND deleted_at IS NULL
 AND (locked_by_report_id IS NULL OR locked_by_report_id=$2::uuid)`, row.ID, reportID, row.RowVersion)
		if err != nil {
			return reporting.ReportDraft{}, err
		}
		if tag.RowsAffected() != 1 {
			return reporting.ReportDraft{}, reporting.ErrReportVersionConflict
		}
		includedData, err := json.Marshal(snapshot.Record)
		if err != nil {
			return reporting.ReportDraft{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO weekly_report_version_records(
 report_version_id,inspection_record_id,source_submission_id,source_revision_no,record_version,included_data_json)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6::jsonb)`, command.VersionID, row.ID, row.SubmissionID,
			row.RevisionNo, row.RowVersion, includedData); err != nil {
			return reporting.ReportDraft{}, err
		}
	}
	updated, err := tx.Exec(ctx, `UPDATE weekly_reports SET status='DRAFT',current_version=$2,
 row_version=CASE WHEN current_version=0 THEN row_version ELSE row_version+1 END,
 finalized_by=NULL,finalized_at=NULL,published_by=NULL,published_at=NULL,
 updated_by=$3::uuid,updated_at=$4 WHERE id=$1::uuid`, reportID, versionNo, command.ActorID, createdAt)
	if err != nil {
		return reporting.ReportDraft{}, err
	}
	if updated.RowsAffected() != 1 {
		return reporting.ReportDraft{}, reporting.ErrReportNotFound
	}
	if err := insertAuditTx(ctx, tx, command.Metadata, audit.Entry{
		Action: audit.ActionReportDraftGenerated, ResourceType: "weeklyReport", ResourceID: reportID,
		Before: map[string]any{"status": status, "versionNo": currentVersion},
		After: map[string]any{"status": reporting.ReportStatusDraft, "versionNo": versionNo,
			"confirmedRecordCount": command.Content.ConfirmedRecordCount, "sourceFingerprint": fingerprintHex,
			"scopeLimited": command.Content.ScopeLimited, "forced": command.ForceReason != ""}, CreatedAt: createdAt,
	}); err != nil {
		return reporting.ReportDraft{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return reporting.ReportDraft{}, err
	}
	newRowVersion := rowVersion
	if currentVersion > 0 {
		newRowVersion++
	}
	return reporting.ReportDraft{ID: reportID, Status: reporting.ReportStatusDraft, RowVersion: newRowVersion,
		VersionNo: versionNo, Content: command.Content, CreatedAt: createdAt}, nil
}

func reportSourceQuery(cells []reporting.ReportCell, semesterID, weekID string) (string, []any) {
	args := []any{semesterID, weekID}
	cellPredicates := make([]string, 0, len(cells))
	for _, cell := range cells {
		args = append(args, cell.DepartmentID, cell.InspectionTypeID)
		departmentParam, typeParam := len(args)-1, len(args)
		cellPredicates = append(cellPredicates, fmt.Sprintf("(ds.department_id IS NOT DISTINCT FROM NULLIF($%d,'')::uuid AND ds.inspection_type_id=$%d::uuid)", departmentParam, typeParam))
	}
	query := `SELECT ir.id::text,ir.row_version,ds.id::text,rev.revision_no,COALESCE(ir.locked_by_report_id::text,'')
FROM department_submissions ds
JOIN department_submission_revisions rev ON rev.submission_id=ds.id AND rev.revision_no=ds.revision_no
JOIN LATERAL unnest(rev.record_ids) AS source_ids(record_id) ON true
JOIN inspection_records ir ON ir.id=source_ids.record_id AND ir.submission_id=ds.id
WHERE ds.semester_id=$1::uuid AND ds.week_id=$2::uuid AND ds.status='CONFIRMED'
 AND ds.submission_kind='RECORDS' AND ir.status='CONFIRMED' AND ir.deleted_at IS NULL
 AND (` + strings.Join(cellPredicates, " OR ") + `)
ORDER BY ir.id
FOR UPDATE OF ir`
	return query, args
}

func queryReportZeroReceipts(ctx context.Context, tx pgx.Tx, cells []reporting.ReportCell, semesterID, weekID string) ([]reportZeroReceipt, error) {
	args := []any{semesterID, weekID}
	cellPredicates := make([]string, 0, len(cells))
	for _, cell := range cells {
		args = append(args, cell.DepartmentID, cell.InspectionTypeID)
		departmentParam, typeParam := len(args)-1, len(args)
		cellPredicates = append(cellPredicates, fmt.Sprintf("(ds.department_id IS NOT DISTINCT FROM NULLIF($%d,'')::uuid AND ds.inspection_type_id=$%d::uuid)", departmentParam, typeParam))
	}
	query := `SELECT ds.id::text,ds.revision_no,COALESCE(ds.department_id::text,''),ds.inspection_type_id::text
FROM department_submissions ds
WHERE ds.semester_id=$1::uuid AND ds.week_id=$2::uuid AND ds.status='CONFIRMED' AND ds.submission_kind='ZERO_RECORD'
 AND (` + strings.Join(cellPredicates, " OR ") + `)
ORDER BY ds.id
FOR UPDATE OF ds`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	receipts := make([]reportZeroReceipt, 0)
	for rows.Next() {
		var receipt reportZeroReceipt
		if err := rows.Scan(&receipt.ID, &receipt.RevisionNo, &receipt.DepartmentID, &receipt.InspectionTypeID); err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return receipts, nil
}

func reportRecordFingerprintPart(record reportSourceRecord) string {
	return fmt.Sprintf("record:%s:%d:%s:%d", record.ID, record.RowVersion, record.SubmissionID, record.RevisionNo)
}

func reportZeroReceiptFingerprintPart(receipt reportZeroReceipt) string {
	return fmt.Sprintf("receipt:%s:%s:%s:%d", receipt.DepartmentID, receipt.InspectionTypeID, receipt.ID, receipt.RevisionNo)
}

func currentReportSourceFingerprint(ctx context.Context, tx pgx.Tx, semesterID, weekID string, content reporting.ReportVersionContent) (string, error) {
	cells := make([]reporting.ReportCell, 0, len(content.Rows))
	for _, row := range content.Rows {
		cells = append(cells, reporting.ReportCell{DepartmentID: row.DepartmentID, InspectionTypeID: row.InspectionTypeID})
	}
	query, args := reportSourceQuery(cells, semesterID, weekID)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0)
	for rows.Next() {
		var item reportSourceRecord
		if err := rows.Scan(&item.ID, &item.RowVersion, &item.SubmissionID, &item.RevisionNo, &item.LockedReportID); err != nil {
			rows.Close()
			return "", err
		}
		parts = append(parts, reportRecordFingerprintPart(item))
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()
	zeroReceipts, err := queryReportZeroReceipts(ctx, tx, cells, semesterID, weekID)
	if err != nil {
		return "", err
	}
	for _, receipt := range zeroReceipts {
		parts = append(parts, reportZeroReceiptFingerprintPart(receipt))
	}
	sort.Strings(parts)
	digest := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(digest[:]), nil
}

// AddReportCorrection appends a report-only correction version. Department
// submission revisions stay immutable; factual changes remain unapplied until
// the source department records a confirmation decision.
func (p *Postgres) AddReportCorrection(ctx context.Context, reportID, actorID string, input reporting.AddReportCorrectionInput,
	permissions []permission.EffectivePermission, metadata audit.Metadata) (reporting.ReportCorrectionResult, error) {
	path, ok := reporting.ReportCorrectionFieldPath(input.Field)
	if !ok {
		return reporting.ReportCorrectionResult{}, reporting.ErrInvalidReportCorrection
	}
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return reporting.ReportCorrectionResult{}, err
	}
	defer tx.Rollback(ctx)
	var state reporting.ReportState
	var currentVersionID, oldContentJSON, sourceFingerprint, forceReason, academicYearID string
	var scopeLimited bool
	err = tx.QueryRow(ctx, `SELECT wr.id::text,wr.semester_id::text,wr.week_id::text,wr.status,wr.current_version,wr.row_version,wr.updated_at,
 v.id::text,v.content_json::text,v.source_fingerprint,v.forced_generation_reason,v.scope_limited,sem.academic_year_id::text
FROM weekly_reports wr JOIN semesters sem ON sem.id=wr.semester_id
JOIN weekly_report_versions v ON v.report_id=wr.id AND v.version_no=wr.current_version
WHERE wr.id=$1::uuid FOR UPDATE OF wr`, reportID).Scan(&state.ID, &state.SemesterID, &state.WeekID, &state.Status,
		&state.VersionNo, &state.RowVersion, &state.UpdatedAt, &currentVersionID, &oldContentJSON, &sourceFingerprint,
		&forceReason, &scopeLimited, &academicYearID)
	if errors.Is(err, pgx.ErrNoRows) {
		return reporting.ReportCorrectionResult{}, reporting.ErrReportNotFound
	}
	if err != nil {
		return reporting.ReportCorrectionResult{}, err
	}
	if input.RowVersion != state.RowVersion {
		return reporting.ReportCorrectionResult{}, reporting.ErrReportVersionConflict
	}
	if state.Status != reporting.ReportStatusDraft && state.Status != reporting.ReportStatusReturned &&
		state.Status != reporting.ReportStatusFinalized && state.Status != reporting.ReportStatusPublished && state.Status != reporting.ReportStatusArchived {
		return reporting.ReportCorrectionResult{}, reporting.ErrInvalidReportTransition
	}
	var beforeJSON, submissionID string
	var inspectionTypeID, departmentID string
	err = tx.QueryRow(ctx, `SELECT COALESCE(version_record.included_data_json #> $3::text[],'null'::jsonb)::text,
 version_record.source_submission_id::text,inspection_record.inspection_type_id::text,COALESCE(inspection_record.department_id::text,'')
FROM weekly_report_version_records version_record
JOIN inspection_records inspection_record ON inspection_record.id=version_record.inspection_record_id
WHERE version_record.report_version_id=$1::uuid AND version_record.inspection_record_id=$2::uuid`, currentVersionID, input.RecordID, path).
		Scan(&beforeJSON, &submissionID, &inspectionTypeID, &departmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return reporting.ReportCorrectionResult{}, reporting.ErrReportCorrectionNotFound
	}
	if err != nil {
		return reporting.ReportCorrectionResult{}, err
	}
	if !permission.AllowsWholeScope(permissions, permission.ActionReportCorrect, permission.Scope{
		AcademicYearID: academicYearID, SemesterID: state.SemesterID, WeekID: state.WeekID,
		InspectionTypeID: inspectionTypeID, DepartmentID: departmentID,
	}) {
		return reporting.ReportCorrectionResult{}, reporting.ErrForbidden
	}
	var beforeValue, afterValue any
	if json.Unmarshal([]byte(beforeJSON), &beforeValue) != nil || json.Unmarshal(input.AfterValue, &afterValue) != nil {
		return reporting.ReportCorrectionResult{}, reporting.ErrInvalidReportCorrection
	}
	if reflect.DeepEqual(beforeValue, afterValue) {
		return reporting.ReportCorrectionResult{}, reporting.ErrInvalidReportCorrection
	}
	var content reporting.ReportVersionContent
	if err := json.Unmarshal([]byte(oldContentJSON), &content); err != nil {
		return reporting.ReportCorrectionResult{}, err
	}
	correctionID, versionID := uuid.NewString(), uuid.NewString()
	status := "APPLIED"
	if input.FactChanged {
		status = "PENDING_DEPARTMENT_CONFIRMATION"
		content.PendingDepartmentReconfirmationCount++
	}
	content.Corrections = append(content.Corrections, reporting.ReportCorrectionSummary{
		ID: correctionID, RecordID: input.RecordID, Field: input.Field, FactChanged: input.FactChanged, Status: status,
	})
	content.GeneratedAt = time.Now().UTC()
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return reporting.ReportCorrectionResult{}, err
	}
	newVersionNo := state.VersionNo + 1
	createdAt := time.Now().UTC()
	if _, err := tx.Exec(ctx, `INSERT INTO weekly_report_versions(
 id,report_id,version_no,content_json,source_fingerprint,forced_generation_reason,scope_limited,created_by,created_at)
 VALUES($1::uuid,$2::uuid,$3,$4::jsonb,$5,$6,$7,$8::uuid,$9)`, versionID, reportID, newVersionNo,
		contentJSON, sourceFingerprint, forceReason, scopeLimited, actorID, createdAt); err != nil {
		return reporting.ReportCorrectionResult{}, err
	}
	afterJSON, err := json.Marshal(map[string]any{"field": input.Field, "value": json.RawMessage(input.AfterValue)})
	if err != nil {
		return reporting.ReportCorrectionResult{}, err
	}
	departmentStatus := "NOT_REQUIRED"
	if input.FactChanged {
		departmentStatus = "REQUIRED"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO weekly_report_corrections(
 id,report_id,report_version_id,inspection_record_id,source_submission_id,before_json,after_json,reason,fact_changed,
 department_reconfirmation_status,corrected_by,corrected_at)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6::jsonb,$7::jsonb,$8,$9,$10,$11::uuid,$12)`,
		correctionID, reportID, versionID, input.RecordID, submissionID, beforeJSON, afterJSON, input.Reason,
		input.FactChanged, departmentStatus, actorID, createdAt); err != nil {
		return reporting.ReportCorrectionResult{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO weekly_report_version_records(
 report_version_id,inspection_record_id,source_submission_id,source_revision_no,record_version,included_data_json)
SELECT $1::uuid,inspection_record_id,source_submission_id,source_revision_no,record_version,
 CASE WHEN inspection_record_id=$3::uuid AND NOT $4 THEN jsonb_set(included_data_json,$5::text[],$6::jsonb,true)
 ELSE included_data_json END
FROM weekly_report_version_records WHERE report_version_id=$2::uuid`, versionID, currentVersionID, input.RecordID,
		input.FactChanged, path, input.AfterValue); err != nil {
		return reporting.ReportCorrectionResult{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE weekly_reports SET status='DRAFT',current_version=$2,row_version=row_version+1,
 finalized_by=NULL,finalized_at=NULL,published_by=NULL,published_at=NULL,updated_by=$3::uuid,updated_at=$4
 WHERE id=$1::uuid AND row_version=$5`, reportID, newVersionNo, actorID, createdAt, state.RowVersion); err != nil {
		return reporting.ReportCorrectionResult{}, err
	}
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{
		Action: audit.ActionReportCorrectionAdded, ResourceType: "weeklyReport", ResourceID: reportID,
		Before: map[string]any{"status": state.Status, "rowVersion": state.RowVersion, "versionNo": state.VersionNo},
		After: map[string]any{"status": reporting.ReportStatusDraft, "rowVersion": state.RowVersion + 1,
			"versionNo": newVersionNo, "recordId": input.RecordID, "field": input.Field,
			"factChanged": input.FactChanged, "departmentReconfirmation": departmentStatus}, CreatedAt: createdAt,
	}); err != nil {
		return reporting.ReportCorrectionResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return reporting.ReportCorrectionResult{}, err
	}
	state.Status, state.VersionNo, state.RowVersion, state.UpdatedAt = reporting.ReportStatusDraft, newVersionNo, state.RowVersion+1, createdAt
	return reporting.ReportCorrectionResult{CorrectionID: correctionID, Report: state}, nil
}

// AddReportAdjustment appends an immutable version after including or excluding
// a confirmed source row. Exclusion affects only the new report version.
func (p *Postgres) AddReportAdjustment(ctx context.Context, reportID, actorID string, input reporting.AddReportAdjustmentInput,
	permissions []permission.EffectivePermission, metadata audit.Metadata) (reporting.ReportAdjustmentResult, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return reporting.ReportAdjustmentResult{}, err
	}
	defer tx.Rollback(ctx)
	var state reporting.ReportState
	var currentVersionID, contentJSON, sourceFingerprint, forceReason, academicYearID string
	var scopeLimited bool
	err = tx.QueryRow(ctx, `SELECT wr.id::text,wr.semester_id::text,wr.week_id::text,wr.status,wr.current_version,wr.row_version,wr.updated_at,
 v.id::text,v.content_json::text,v.source_fingerprint,v.forced_generation_reason,v.scope_limited,sem.academic_year_id::text
FROM weekly_reports wr JOIN semesters sem ON sem.id=wr.semester_id
JOIN weekly_report_versions v ON v.report_id=wr.id AND v.version_no=wr.current_version
WHERE wr.id=$1::uuid FOR UPDATE OF wr`, reportID).Scan(&state.ID, &state.SemesterID, &state.WeekID,
		&state.Status, &state.VersionNo, &state.RowVersion, &state.UpdatedAt, &currentVersionID, &contentJSON,
		&sourceFingerprint, &forceReason, &scopeLimited, &academicYearID)
	if errors.Is(err, pgx.ErrNoRows) {
		return reporting.ReportAdjustmentResult{}, reporting.ErrReportNotFound
	}
	if err != nil {
		return reporting.ReportAdjustmentResult{}, err
	}
	if state.RowVersion != input.RowVersion {
		return reporting.ReportAdjustmentResult{}, reporting.ErrReportVersionConflict
	}
	if state.Status != reporting.ReportStatusDraft && state.Status != reporting.ReportStatusReturned &&
		state.Status != reporting.ReportStatusFinalized && state.Status != reporting.ReportStatusPublished &&
		state.Status != reporting.ReportStatusArchived {
		return reporting.ReportAdjustmentResult{}, reporting.ErrInvalidReportTransition
	}
	var content reporting.ReportVersionContent
	if err := json.Unmarshal([]byte(contentJSON), &content); err != nil {
		return reporting.ReportAdjustmentResult{}, err
	}
	currentFingerprint, err := currentReportSourceFingerprint(ctx, tx, state.SemesterID, state.WeekID, content)
	if err != nil {
		return reporting.ReportAdjustmentResult{}, err
	}
	if currentFingerprint != sourceFingerprint {
		return reporting.ReportAdjustmentResult{}, reporting.ErrReportSourceChanged
	}

	var sourceSubmissionID, lockedReportID, departmentID, inspectionTypeID string
	var sourceRevisionNo, recordVersion int
	var recordDataJSON string
	if input.Operation == "EXCLUDE" {
		err = tx.QueryRow(ctx, `SELECT version_record.source_submission_id::text,version_record.source_revision_no,
 version_record.record_version,version_record.included_data_json::text,COALESCE(record.department_id::text,''),
 record.inspection_type_id::text,COALESCE(record.locked_by_report_id::text,'')
FROM weekly_report_version_records version_record
JOIN inspection_records record ON record.id=version_record.inspection_record_id
WHERE version_record.report_version_id=$1::uuid AND version_record.inspection_record_id=$2::uuid`,
			currentVersionID, input.RecordID).Scan(&sourceSubmissionID, &sourceRevisionNo, &recordVersion, &recordDataJSON,
			&departmentID, &inspectionTypeID, &lockedReportID)
	} else if input.Operation == "INCLUDE" {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM weekly_report_version_records
WHERE report_version_id=$1::uuid AND inspection_record_id=$2::uuid)`, currentVersionID, input.RecordID).Scan(&exists); err != nil {
			return reporting.ReportAdjustmentResult{}, err
		}
		if exists {
			return reporting.ReportAdjustmentResult{}, reporting.ErrInvalidReportAdjustment
		}
		err = tx.QueryRow(ctx, `SELECT record.row_version,submission.id::text,revision.revision_no,
 COALESCE(record.department_id::text,''),record.inspection_type_id::text,COALESCE(record.locked_by_report_id::text,'')
FROM inspection_records record
JOIN department_submissions submission ON submission.id=record.submission_id
JOIN department_submission_revisions revision ON revision.submission_id=submission.id
 AND revision.revision_no=submission.revision_no AND record.id=ANY(revision.record_ids)
WHERE record.id=$1::uuid AND record.semester_id=$2::uuid AND record.week_id=$3::uuid
 AND record.status='CONFIRMED' AND record.deleted_at IS NULL AND submission.status='CONFIRMED'
 AND submission.submission_kind='RECORDS' FOR UPDATE OF record,submission`, input.RecordID, state.SemesterID, state.WeekID).
			Scan(&recordVersion, &sourceSubmissionID, &sourceRevisionNo, &departmentID, &inspectionTypeID, &lockedReportID)
	} else {
		return reporting.ReportAdjustmentResult{}, reporting.ErrInvalidReportAdjustment
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if input.Operation == "EXCLUDE" {
			return reporting.ReportAdjustmentResult{}, reporting.ErrReportCorrectionNotFound
		}
		return reporting.ReportAdjustmentResult{}, reporting.ErrReportCorrectionNotFound
	}
	if err != nil {
		return reporting.ReportAdjustmentResult{}, err
	}
	if lockedReportID != "" && lockedReportID != reportID {
		return reporting.ReportAdjustmentResult{}, reporting.ErrReportVersionConflict
	}
	if !permission.AllowsWholeScope(permissions, permission.ActionReportCorrect, permission.Scope{
		AcademicYearID: academicYearID, SemesterID: state.SemesterID, WeekID: state.WeekID,
		InspectionTypeID: inspectionTypeID, DepartmentID: departmentID,
	}) {
		return reporting.ReportAdjustmentResult{}, reporting.ErrForbidden
	}
	cellFound := false
	for _, row := range content.Rows {
		if row.InspectionTypeID == inspectionTypeID && row.DepartmentID == departmentID {
			cellFound = true
			break
		}
	}
	if !cellFound {
		return reporting.ReportAdjustmentResult{}, reporting.ErrForbidden
	}
	if input.Operation == "INCLUDE" {
		var previousData, previousSubmissionID string
		var previousRevisionNo, previousRecordVersion int
		previousErr := tx.QueryRow(ctx, `SELECT source_submission_id::text,source_revision_no,record_version,included_data_json::text
FROM weekly_report_version_records version_record
JOIN weekly_report_versions version ON version.id=version_record.report_version_id
WHERE version.report_id=$1::uuid AND version_record.inspection_record_id=$2::uuid
ORDER BY version.version_no DESC LIMIT 1`, reportID, input.RecordID).Scan(
			&previousSubmissionID, &previousRevisionNo, &previousRecordVersion, &previousData)
		if previousErr == nil {
			sourceSubmissionID, sourceRevisionNo, recordVersion, recordDataJSON = previousSubmissionID, previousRevisionNo, previousRecordVersion, previousData
		} else if errors.Is(previousErr, pgx.ErrNoRows) {
			// The source row may be newly confirmed. It can be added only after the
			// report is regenerated from current sources, so a matching fingerprint
			// without a prior snapshot is an inconsistent source state.
			return reporting.ReportAdjustmentResult{}, reporting.ErrReportSourceChanged
		} else {
			return reporting.ReportAdjustmentResult{}, previousErr
		}
	}
	var record reporting.Record
	if err := json.Unmarshal([]byte(recordDataJSON), &record); err != nil {
		return reporting.ReportAdjustmentResult{}, err
	}
	if input.Operation == "INCLUDE" {
		if _, err := tx.Exec(ctx, `UPDATE inspection_records SET locked_by_report_id=$2::uuid
 WHERE id=$1::uuid AND row_version=$3 AND status='CONFIRMED' AND deleted_at IS NULL
 AND (locked_by_report_id IS NULL OR locked_by_report_id=$2::uuid)`, input.RecordID, reportID, recordVersion); err != nil {
			return reporting.ReportAdjustmentResult{}, err
		}
	}
	change := 1
	if input.Operation == "EXCLUDE" {
		change = -1
	}
	updatedCount := false
	for index := range content.Rows {
		if content.Rows[index].InspectionTypeID == inspectionTypeID && content.Rows[index].DepartmentID == departmentID {
			if content.Rows[index].RecordCount+change < 0 {
				return reporting.ReportAdjustmentResult{}, reporting.ErrInvalidReportAdjustment
			}
			content.Rows[index].RecordCount += change
			updatedCount = true
			break
		}
	}
	if !updatedCount || content.ConfirmedRecordCount+change < 0 {
		return reporting.ReportAdjustmentResult{}, reporting.ErrInvalidReportAdjustment
	}
	content.ConfirmedRecordCount += change
	adjustmentID, versionID, createdAt := uuid.NewString(), uuid.NewString(), time.Now().UTC()
	content.Adjustments = append(content.Adjustments, reporting.ReportAdjustmentSummary{
		ID: adjustmentID, RecordID: input.RecordID, Operation: input.Operation, Reason: input.Reason,
		Record: record, AdjustedBy: actorID, AdjustedAt: createdAt,
	})
	content.GeneratedAt = createdAt
	contentBytes, err := json.Marshal(content)
	if err != nil {
		return reporting.ReportAdjustmentResult{}, err
	}
	newVersionNo := state.VersionNo + 1
	if _, err := tx.Exec(ctx, `INSERT INTO weekly_report_versions(
 id,report_id,version_no,content_json,source_fingerprint,forced_generation_reason,scope_limited,created_by,created_at)
 VALUES($1::uuid,$2::uuid,$3,$4::jsonb,$5,$6,$7,$8::uuid,$9)`, versionID, reportID, newVersionNo,
		contentBytes, sourceFingerprint, forceReason, scopeLimited, actorID, createdAt); err != nil {
		return reporting.ReportAdjustmentResult{}, err
	}
	var copyQuery string
	if input.Operation == "EXCLUDE" {
		copyQuery = `INSERT INTO weekly_report_version_records(
 report_version_id,inspection_record_id,source_submission_id,source_revision_no,record_version,included_data_json)
SELECT $1::uuid,inspection_record_id,source_submission_id,source_revision_no,record_version,included_data_json
FROM weekly_report_version_records WHERE report_version_id=$2::uuid AND inspection_record_id<>$3::uuid`
		if _, err := tx.Exec(ctx, copyQuery, versionID, currentVersionID, input.RecordID); err != nil {
			return reporting.ReportAdjustmentResult{}, err
		}
	} else {
		copyQuery = `INSERT INTO weekly_report_version_records(
 report_version_id,inspection_record_id,source_submission_id,source_revision_no,record_version,included_data_json)
SELECT $1::uuid,inspection_record_id,source_submission_id,source_revision_no,record_version,included_data_json
FROM weekly_report_version_records WHERE report_version_id=$2::uuid`
		if _, err := tx.Exec(ctx, copyQuery, versionID, currentVersionID); err != nil {
			return reporting.ReportAdjustmentResult{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO weekly_report_version_records(
 report_version_id,inspection_record_id,source_submission_id,source_revision_no,record_version,included_data_json)
VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6::jsonb)`, versionID, input.RecordID, sourceSubmissionID,
			sourceRevisionNo, recordVersion, recordDataJSON); err != nil {
			return reporting.ReportAdjustmentResult{}, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO weekly_report_version_adjustments(
 id,report_id,report_version_id,inspection_record_id,operation,reason,record_data_json,adjusted_by,adjusted_at)
VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7::jsonb,$8::uuid,$9)`, adjustmentID, reportID, versionID,
		input.RecordID, input.Operation, input.Reason, recordDataJSON, actorID, createdAt); err != nil {
		return reporting.ReportAdjustmentResult{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE weekly_reports SET status='DRAFT',current_version=$2,row_version=row_version+1,
 finalized_by=NULL,finalized_at=NULL,published_by=NULL,published_at=NULL,updated_by=$3::uuid,updated_at=$4
 WHERE id=$1::uuid AND row_version=$5`, reportID, newVersionNo, actorID, createdAt, state.RowVersion); err != nil {
		return reporting.ReportAdjustmentResult{}, err
	}
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{
		Action: audit.ActionReportAdjustmentAdded, ResourceType: "weeklyReport", ResourceID: reportID,
		Before: map[string]any{"status": state.Status, "rowVersion": state.RowVersion, "versionNo": state.VersionNo},
		After: map[string]any{"status": reporting.ReportStatusDraft, "rowVersion": state.RowVersion + 1,
			"versionNo": newVersionNo, "recordId": input.RecordID, "operation": input.Operation,
			"confirmedRecordCount": content.ConfirmedRecordCount}, CreatedAt: createdAt,
	}); err != nil {
		return reporting.ReportAdjustmentResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return reporting.ReportAdjustmentResult{}, err
	}
	state.Status, state.VersionNo, state.RowVersion, state.UpdatedAt = reporting.ReportStatusDraft, newVersionNo, state.RowVersion+1, createdAt
	return reporting.ReportAdjustmentResult{AdjustmentID: adjustmentID, Report: state}, nil
}

// DecideReportCorrection records the source department's append-only response.
// A confirmed fact is applied only to a newly created report version.
func (p *Postgres) DecideReportCorrection(ctx context.Context, correctionID, actorID string, input reporting.DecideReportCorrectionInput,
	permissions []permission.EffectivePermission, metadata audit.Metadata) (reporting.ReportState, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return reporting.ReportState{}, err
	}
	defer tx.Rollback(ctx)
	var reportID, correctionVersionID, sourceSubmissionID, recordID, academicYearID string
	var semesterID, weekID, inspectionTypeID, departmentID string
	var afterJSON []byte
	err = tx.QueryRow(ctx, `SELECT correction.report_id::text,correction.report_version_id::text,
 correction.source_submission_id::text,correction.inspection_record_id::text,correction.after_json::text,
 submission.semester_id::text,submission.week_id::text,submission.inspection_type_id::text,COALESCE(submission.department_id::text,''),sem.academic_year_id::text
FROM weekly_report_corrections correction
JOIN department_submissions submission ON submission.id=correction.source_submission_id
JOIN semesters sem ON sem.id=submission.semester_id
WHERE correction.id=$1::uuid AND correction.department_reconfirmation_status='REQUIRED'`, correctionID).
		Scan(&reportID, &correctionVersionID, &sourceSubmissionID, &recordID, &afterJSON, &semesterID, &weekID,
			&inspectionTypeID, &departmentID, &academicYearID)
	if errors.Is(err, pgx.ErrNoRows) {
		var decided bool
		if queryErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM weekly_report_correction_responses WHERE correction_id=$1::uuid)`, correctionID).Scan(&decided); queryErr != nil {
			return reporting.ReportState{}, queryErr
		}
		if decided {
			return reporting.ReportState{}, reporting.ErrReportCorrectionDecided
		}
		return reporting.ReportState{}, reporting.ErrReportCorrectionNotFound
	}
	if err != nil {
		return reporting.ReportState{}, err
	}
	if !permission.AllowsWholeScope(permissions, permission.ActionInspectionReview, permission.Scope{
		AcademicYearID: academicYearID, SemesterID: semesterID, WeekID: weekID,
		InspectionTypeID: inspectionTypeID, DepartmentID: departmentID,
	}) {
		return reporting.ReportState{}, reporting.ErrForbidden
	}
	var state reporting.ReportState
	var currentVersionID, contentJSON, sourceFingerprint, forceReason string
	var scopeLimited bool
	err = tx.QueryRow(ctx, `SELECT wr.id::text,wr.semester_id::text,wr.week_id::text,wr.status,wr.current_version,wr.row_version,wr.updated_at,
 v.id::text,v.content_json::text,v.source_fingerprint,v.forced_generation_reason,v.scope_limited
FROM weekly_reports wr JOIN weekly_report_versions v ON v.report_id=wr.id AND v.version_no=wr.current_version
WHERE wr.id=$1::uuid FOR UPDATE OF wr`, reportID).Scan(&state.ID, &state.SemesterID, &state.WeekID,
		&state.Status, &state.VersionNo, &state.RowVersion, &state.UpdatedAt, &currentVersionID, &contentJSON,
		&sourceFingerprint, &forceReason, &scopeLimited)
	if errors.Is(err, pgx.ErrNoRows) {
		return reporting.ReportState{}, reporting.ErrReportNotFound
	}
	if err != nil {
		return reporting.ReportState{}, err
	}
	if state.RowVersion != input.RowVersion {
		return reporting.ReportState{}, reporting.ErrReportVersionConflict
	}
	if state.Status != reporting.ReportStatusDraft {
		return reporting.ReportState{}, reporting.ErrInvalidReportTransition
	}
	var alreadyDecided bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM weekly_report_correction_responses WHERE correction_id=$1::uuid)`, correctionID).Scan(&alreadyDecided); err != nil {
		return reporting.ReportState{}, err
	}
	if alreadyDecided {
		return reporting.ReportState{}, reporting.ErrReportCorrectionDecided
	}
	var content reporting.ReportVersionContent
	if err := json.Unmarshal([]byte(contentJSON), &content); err != nil {
		return reporting.ReportState{}, err
	}
	var correctionSummaryIndex = -1
	for index, correction := range content.Corrections {
		if correction.ID == correctionID && correction.Status == "PENDING_DEPARTMENT_CONFIRMATION" {
			correctionSummaryIndex = index
			break
		}
	}
	if correctionSummaryIndex < 0 || content.PendingDepartmentReconfirmationCount < 1 {
		return reporting.ReportState{}, reporting.ErrReportCorrectionDecided
	}
	var after map[string]json.RawMessage
	if err := json.Unmarshal(afterJSON, &after); err != nil {
		return reporting.ReportState{}, err
	}
	var field string
	if err := json.Unmarshal(after["field"], &field); err != nil {
		return reporting.ReportState{}, err
	}
	path, ok := reporting.ReportCorrectionFieldPath(field)
	if !ok {
		return reporting.ReportState{}, reporting.ErrInvalidReportCorrection
	}
	if _, err := tx.Exec(ctx, `INSERT INTO weekly_report_correction_responses(id,correction_id,decision,comment,actor_id)
 VALUES($1::uuid,$2::uuid,$3,$4,$5::uuid)`, uuid.NewString(), correctionID, input.Decision,
		strings.TrimSpace(input.Comment), actorID); err != nil {
		return reporting.ReportState{}, err
	}
	if input.Decision == "CONFIRMED" {
		content.Corrections[correctionSummaryIndex].Status = "CONFIRMED"
	} else {
		content.Corrections[correctionSummaryIndex].Status = "REJECTED"
	}
	content.PendingDepartmentReconfirmationCount--
	content.GeneratedAt = time.Now().UTC()
	contentBytes, err := json.Marshal(content)
	if err != nil {
		return reporting.ReportState{}, err
	}
	newVersionID, newVersionNo, createdAt := uuid.NewString(), state.VersionNo+1, time.Now().UTC()
	if _, err := tx.Exec(ctx, `INSERT INTO weekly_report_versions(
 id,report_id,version_no,content_json,source_fingerprint,forced_generation_reason,scope_limited,created_by,created_at)
 VALUES($1::uuid,$2::uuid,$3,$4::jsonb,$5,$6,$7,$8::uuid,$9)`, newVersionID, reportID, newVersionNo,
		contentBytes, sourceFingerprint, forceReason, scopeLimited, actorID, createdAt); err != nil {
		return reporting.ReportState{}, err
	}
	var copyQuery string
	if input.Decision == "CONFIRMED" {
		copyQuery = `INSERT INTO weekly_report_version_records(
 report_version_id,inspection_record_id,source_submission_id,source_revision_no,record_version,included_data_json)
SELECT $1::uuid,inspection_record_id,source_submission_id,source_revision_no,record_version,
 CASE WHEN inspection_record_id=$3::uuid THEN jsonb_set(included_data_json,$4::text[],$5::jsonb,true)
 ELSE included_data_json END
FROM weekly_report_version_records WHERE report_version_id=$2::uuid`
	} else {
		copyQuery = `INSERT INTO weekly_report_version_records(
 report_version_id,inspection_record_id,source_submission_id,source_revision_no,record_version,included_data_json)
SELECT $1::uuid,inspection_record_id,source_submission_id,source_revision_no,record_version,included_data_json
FROM weekly_report_version_records WHERE report_version_id=$2::uuid`
	}
	if input.Decision == "CONFIRMED" {
		if _, err := tx.Exec(ctx, copyQuery, newVersionID, currentVersionID, recordID, path, after["value"]); err != nil {
			return reporting.ReportState{}, err
		}
	} else if _, err := tx.Exec(ctx, copyQuery, newVersionID, currentVersionID); err != nil {
		return reporting.ReportState{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE weekly_reports SET current_version=$2,row_version=row_version+1,updated_by=$3::uuid,updated_at=$4
 WHERE id=$1::uuid AND row_version=$5`, reportID, newVersionNo, actorID, createdAt, state.RowVersion); err != nil {
		return reporting.ReportState{}, err
	}
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{
		Action: audit.ActionReportCorrectionAdded, ResourceType: "weeklyReport", ResourceID: reportID,
		Before: map[string]any{"rowVersion": state.RowVersion, "versionNo": state.VersionNo},
		After: map[string]any{"rowVersion": state.RowVersion + 1, "versionNo": newVersionNo,
			"correctionId": correctionID, "departmentDecision": input.Decision, "sourceSubmissionId": sourceSubmissionID}, CreatedAt: createdAt,
	}); err != nil {
		return reporting.ReportState{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return reporting.ReportState{}, err
	}
	state.VersionNo, state.RowVersion, state.Status, state.UpdatedAt = newVersionNo, state.RowVersion+1, reporting.ReportStatusDraft, createdAt
	_ = correctionVersionID
	return state, nil
}

// TransitionReport writes lifecycle history and its audit atomically.
func (p *Postgres) TransitionReport(ctx context.Context, reportID, actorID, action string,
	input reporting.ReportTransitionInput, permissions []permission.EffectivePermission, metadata audit.Metadata) (reporting.ReportState, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return reporting.ReportState{}, err
	}
	defer tx.Rollback(ctx)
	var current reporting.ReportState
	var contentJSON []byte
	var versionID, sourceFingerprint, academicYearID string
	err = tx.QueryRow(ctx, `SELECT wr.id::text,wr.semester_id::text,wr.week_id::text,wr.status,wr.current_version,wr.row_version,wr.updated_at,
 v.id::text,v.content_json::text,v.source_fingerprint,sem.academic_year_id::text
FROM weekly_reports wr JOIN semesters sem ON sem.id=wr.semester_id
JOIN weekly_report_versions v ON v.report_id=wr.id AND v.version_no=wr.current_version
WHERE wr.id=$1::uuid FOR UPDATE OF wr`, reportID).Scan(&current.ID, &current.SemesterID, &current.WeekID,
		&current.Status, &current.VersionNo, &current.RowVersion, &current.UpdatedAt, &versionID, &contentJSON, &sourceFingerprint, &academicYearID)
	if errors.Is(err, pgx.ErrNoRows) {
		return reporting.ReportState{}, reporting.ErrReportNotFound
	}
	if err != nil {
		return reporting.ReportState{}, err
	}
	if input.RowVersion != current.RowVersion {
		return reporting.ReportState{}, reporting.ErrReportVersionConflict
	}
	var content reporting.ReportVersionContent
	if err := json.Unmarshal(contentJSON, &content); err != nil {
		return reporting.ReportState{}, err
	}
	permissionAction := map[string]string{
		reporting.ReportActionSubmit:    permission.ActionReportGenerate,
		reporting.ReportActionApprove:   permission.ActionReportReview,
		reporting.ReportActionReturn:    permission.ActionReportReturn,
		reporting.ReportActionFinalized: permission.ActionReportFinalize,
	}[action]
	if permissionAction == "" || len(content.Rows) == 0 {
		return reporting.ReportState{}, reporting.ErrInvalidReportRequest
	}
	for _, row := range content.Rows {
		if !permission.AllowsWholeScope(permissions, permissionAction, permission.Scope{
			AcademicYearID: academicYearID, SemesterID: current.SemesterID, WeekID: current.WeekID,
			InspectionTypeID: row.InspectionTypeID, DepartmentID: row.DepartmentID,
		}) {
			return reporting.ReportState{}, reporting.ErrForbidden
		}
	}
	if action == reporting.ReportActionSubmit && content.PendingDepartmentReconfirmationCount > 0 {
		return reporting.ReportState{}, reporting.ErrReportReconfirmationPending
	}
	if action == reporting.ReportActionSubmit || action == reporting.ReportActionApprove || action == reporting.ReportActionFinalized {
		currentFingerprint, fingerprintErr := currentReportSourceFingerprint(ctx, tx, current.SemesterID, current.WeekID, content)
		if fingerprintErr != nil {
			return reporting.ReportState{}, fingerprintErr
		}
		if currentFingerprint != sourceFingerprint {
			return reporting.ReportState{}, reporting.ErrReportSourceChanged
		}
	}
	if action == reporting.ReportActionReturn && strings.TrimSpace(input.Comment) == "" {
		return reporting.ReportState{}, reporting.ErrInvalidReportRequest
	}
	nextStatus, err := reporting.NextReportStatus(current.Status, action)
	if err != nil {
		return reporting.ReportState{}, err
	}
	updatedAt := time.Now().UTC()
	finalized := nextStatus == reporting.ReportStatusFinalized
	tag, err := tx.Exec(ctx, `UPDATE weekly_reports SET status=$2,row_version=row_version+1,updated_by=$3::uuid,updated_at=$4,
 finalized_by=CASE WHEN $6 THEN $3::uuid ELSE finalized_by END,
 finalized_at=CASE WHEN $6 THEN $4 ELSE finalized_at END
 WHERE id=$1::uuid AND row_version=$5`, reportID, nextStatus, actorID, updatedAt, input.RowVersion, finalized)
	if err != nil {
		return reporting.ReportState{}, err
	}
	if tag.RowsAffected() != 1 {
		return reporting.ReportState{}, reporting.ErrReportVersionConflict
	}
	if _, err := tx.Exec(ctx, `INSERT INTO weekly_report_reviews(id,report_id,report_version_id,action,comment,actor_id,created_at)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6::uuid,$7)`, uuid.NewString(), reportID, versionID,
		action, strings.TrimSpace(input.Comment), actorID, updatedAt); err != nil {
		return reporting.ReportState{}, err
	}
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{
		Action: audit.ActionReportWorkflowChanged, ResourceType: "weeklyReport", ResourceID: reportID,
		Before: map[string]any{"status": current.Status, "rowVersion": current.RowVersion, "versionNo": current.VersionNo},
		After: map[string]any{"status": nextStatus, "rowVersion": current.RowVersion + 1,
			"versionNo": current.VersionNo, "action": action, "comment": strings.TrimSpace(input.Comment)}, CreatedAt: updatedAt,
	}); err != nil {
		return reporting.ReportState{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return reporting.ReportState{}, err
	}
	current.Status, current.RowVersion, current.UpdatedAt = nextStatus, current.RowVersion+1, updatedAt
	return current, nil
}
