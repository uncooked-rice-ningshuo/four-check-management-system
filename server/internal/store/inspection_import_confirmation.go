package store

import (
	"context"
	"encoding/json"
	"errors"

	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func inspectionImportMatches(record importjob.InspectionImportRecord, input importjob.ConfirmBatchInput) bool {
	if _, err := uuid.Parse(record.ID); err != nil {
		return false
	}
	return record.SchemaVersion == 1 && record.CreatedBy == input.Scope.ActorID && record.ImportBatchID == input.BatchID && record.Input.SemesterID == input.Scope.SemesterID && record.Input.WeekID == input.Scope.WeekID && record.Input.InspectionTypeID == input.Scope.InspectionTypeID && record.Input.DepartmentID == input.Scope.DepartmentID
}

func prepareInspectionImport(ctx context.Context, tx pgx.Tx, locked inspection.InspectionContext, row importjob.InspectionImportRecord, input importjob.ConfirmBatchInput) (inspection.InspectionRecord, error) {
	if row.TypeCode != locked.TypeCode || (row.TypeCode != inspection.InspectionTypeEveningSelfStudy && row.TypeCode != inspection.InspectionTypeMorning && row.TypeCode != inspection.InspectionTypeLateReturn && row.TypeCode != inspection.InspectionTypeDormitoryHygiene && row.TypeCode != inspection.InspectionTypeClassroomHygiene) || !inspectionImportMatches(row, input) {
		return inspection.InspectionRecord{}, importjob.ErrBatchScopeMismatch
	}
	details := row.Input.Details
	if locked.TypeCode == inspection.InspectionTypeEveningSelfStudy {
		if locked.AttendancePenaltyMultiplier == nil {
			return inspection.InspectionRecord{}, inspection.ErrAttendancePenaltyUnconfigured
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(details, &fields); err != nil || fields == nil {
			return inspection.InspectionRecord{}, importjob.ErrPreviewStale
		}
		if _, exists := fields["period"]; exists {
			return inspection.InspectionRecord{}, importjob.ErrPreviewStale
		}
		if _, exists := fields["absenceHours"]; exists {
			return inspection.InspectionRecord{}, importjob.ErrPreviewStale
		}
		fields["period"] = json.RawMessage(`"9-10"`)
		var err error
		details, err = json.Marshal(fields)
		if err != nil {
			return inspection.InspectionRecord{}, err
		}
	}
	validated, issues := inspection.ValidateDetails(locked.TypeCode, locked.DetailContext, details)
	if len(issues) > 0 {
		return inspection.InspectionRecord{}, importjob.ErrPreviewStale
	}
	record := inspection.InspectionRecord{ID: row.ID, SemesterID: row.Input.SemesterID, WeekID: row.Input.WeekID, InspectionTypeID: row.Input.InspectionTypeID, DepartmentID: row.Input.DepartmentID, TypeCode: row.TypeCode, Details: validated.Details, Normalized: validated, Status: inspection.RecordStatusDraft, Source: inspection.RecordSourceExcel, RowVersion: 1, CreatedBy: row.CreatedBy, ImportBatchID: row.ImportBatchID}
	if locked.TypeCode == inspection.InspectionTypeEveningSelfStudy {
		evening, ok := record.Details.(*inspection.EveningSelfStudyDetails)
		if !ok {
			return inspection.InspectionRecord{}, importjob.ErrPreviewStale
		}
		evening.PenaltyMultiplier = locked.AttendancePenaltyMultiplier
		penaltyCount := *locked.AttendancePenaltyMultiplier
		evening.PenaltyCount = &penaltyCount
		record.PenaltyMultiplier = locked.AttendancePenaltyMultiplier
	}
	// The API reauthorizes the whole batch before confirmation. Every reference
	// must remain inside that exact scope; do not manufacture a permission grant.
	current, err := (&inspectionTx{tx: tx}).lockReferences(ctx, record, locked, func(scope permission.Scope) bool {
		return scope.SemesterID == input.Scope.SemesterID && scope.WeekID == input.Scope.WeekID && scope.InspectionTypeID == input.Scope.InspectionTypeID && scope.DepartmentID == input.Scope.DepartmentID && (input.Scope.ClassID == "" || scope.ClassID == input.Scope.ClassID) && (input.Scope.Grade == "" || scope.Grade == input.Scope.Grade)
	})
	if errors.Is(err, inspection.ErrInvalidAttendanceRecord) {
		return record, importjob.ErrPreviewStale
	}
	if errors.Is(err, inspection.ErrScopeMismatch) {
		return record, importjob.ErrBatchScopeMismatch
	}
	if err != nil {
		return record, err
	}
	if len(current.Classes) != len(row.References.Classes) || len(current.Students) != len(row.References.Students) || len(current.Classes) == 0 {
		return record, importjob.ErrPreviewStale
	}
	if row.TypeCode != inspection.InspectionTypeDormitoryHygiene && row.TypeCode != inspection.InspectionTypeClassroomHygiene && (len(current.Classes) != 1 || len(current.Students) != 1) {
		return record, importjob.ErrPreviewStale
	}
	// Compare identities and relationships independently of preview ordering.
	classSnapshots := map[string]inspection.ClassReference{}
	for _, value := range row.References.Classes {
		classSnapshots[value.ID] = value
	}
	studentSnapshots := map[string]inspection.StudentReference{}
	for _, value := range row.References.Students {
		studentSnapshots[value.ID] = value
	}
	if len(classSnapshots) != len(current.Classes) || len(studentSnapshots) != len(current.Students) {
		return record, importjob.ErrPreviewStale
	}
	for _, value := range current.Classes {
		previous, ok := classSnapshots[value.ID]
		if !ok || previous.Grade != value.Grade {
			return record, importjob.ErrPreviewStale
		}
	}
	for _, value := range current.Students {
		previous, ok := studentSnapshots[value.ID]
		if !ok || previous.ClassID != value.ClassID {
			return record, importjob.ErrPreviewStale
		}
	}
	// Retain the previewed names; reference changes never silently replace the
	// reviewed snapshot. Identity, membership and grade were rechecked above.
	record.References = row.References
	return record, nil
}
