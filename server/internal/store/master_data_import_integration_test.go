package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/masterdata"
	"four-check-management-system/server/internal/masterdataimport"
	"github.com/google/uuid"
)

func TestMasterDataImportBatchIsAtomicAndIdempotentAgainstPostgres(t *testing.T) {
	database := openTestPostgres(t)
	ctx := context.Background()
	actorID := uuid.NewString()
	unique := uuid.NewString()[:8]
	if _, err := database.Pool.Exec(ctx, `INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password)
		VALUES($1,$2,'基础数据导入测试','unused','ACTIVE','SYSTEM','SUPER_ADMIN',true,false)`, actorID, "md-import-"+unique); err != nil {
		t.Fatal(err)
	}
	seedFile := func() string {
		fileID := uuid.NewString()
		if _, err := database.Pool.Exec(ctx, `INSERT INTO files(id,bucket,object_key,original_name,size_bytes,sha256,status,retention_type,created_by)
			VALUES($1,'test',$2,'source.xlsx',1,$3,'AVAILABLE','IMPORT_SOURCE',$4)`, fileID, "master-data-import/"+fileID, strings.Repeat("a", 64), actorID); err != nil {
			t.Fatal(err)
		}
		return fileID
	}
	mutation := audit.Metadata{ActorID: actorID, RequestID: "master-data-import-test"}
	genderOptions, err := database.EnabledDictionaryOptions(ctx, "GENDER")
	if err != nil {
		t.Fatal(err)
	}
	if !containsDictionaryOption(genderOptions, "MALE", "男") {
		t.Fatalf("enabled gender options omitted the persisted value/label pair: %+v", genderOptions)
	}
	statusOptions, err := database.EnabledDictionaryOptions(ctx, "ENROLLMENT_STATUS")
	if err != nil {
		t.Fatal(err)
	}
	if !containsDictionaryOption(statusOptions, "ENROLLED", "在籍") {
		t.Fatalf("enabled enrollment status options omitted the persisted value/label pair: %+v", statusOptions)
	}
	duplicateName := "既有院系-" + unique
	if _, err := database.Pool.Exec(ctx, `INSERT INTO faculties(id,name,enabled) VALUES($1,$2,false)`, uuid.NewString(), duplicateName); err != nil {
		t.Fatal(err)
	}
	found, err := database.FindMasterDataImportRecords(ctx, masterdata.KindFaculty, []masterdataimport.LookupKey{{Name: duplicateName}}, false)
	if err != nil || len(found) != 0 {
		t.Fatalf("disabled record in active reference lookup = %v, %v", found, err)
	}
	found, err = database.FindMasterDataImportRecords(ctx, masterdata.KindFaculty, []masterdataimport.LookupKey{{Name: duplicateName}}, true)
	if err != nil || len(found) != 1 || found[0].Enabled {
		t.Fatalf("disabled record in duplicate lookup = %v, %v", found, err)
	}
	gradeID := uuid.NewString()
	if _, err := database.Pool.Exec(ctx, `INSERT INTO grades(id,name) VALUES($1,'2099级')`, gradeID); err != nil {
		t.Fatal(err)
	}
	dictionaryTx, err := database.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateImportRelationsTx(ctx, dictionaryTx, masterdata.Record{Kind: masterdata.KindStaff, Gender: "UNKNOWN"}); err != nil {
		dictionaryTx.Rollback(ctx)
		t.Fatalf("staff gender dictionary lock: %v", err)
	}
	if err := validateImportRelationsTx(ctx, dictionaryTx, masterdata.Record{
		Kind: masterdata.KindStudent, GradeID: gradeID, Gender: "UNKNOWN", EnrollmentStatus: "ENROLLED",
	}); err != nil {
		dictionaryTx.Rollback(ctx)
		t.Fatalf("student grade/status dictionary locks: %v", err)
	}
	if err := dictionaryTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	firstID, secondID := uuid.NewString(), uuid.NewString()
	failedBatch := masterdataimport.Batch{
		ID: uuid.NewString(), Kind: masterdata.KindFaculty, Status: masterdataimport.StatusPreviewReady, TemplateVersion: 1,
		SourceFileID: seedFile(), TotalRows: 2, ValidRows: 2, Version: 1, CreatedBy: actorID, CreatedAt: time.Now().UTC(),
		Rows: []masterdataimport.PreviewRow{
			{RowNo: 5, Values: map[string]string{"name": "atomic-first-" + unique}, Status: masterdataimport.RowValid, Errors: []masterdataimport.RowError{}, Record: &masterdata.Record{ID: firstID, Kind: masterdata.KindFaculty, Name: "atomic-first-" + unique, Enabled: true}},
			{RowNo: 6, Values: map[string]string{"name": duplicateName}, Status: masterdataimport.RowValid, Errors: []masterdataimport.RowError{}, Record: &masterdata.Record{ID: secondID, Kind: masterdata.KindFaculty, Name: duplicateName, Enabled: true}},
		},
	}
	if err := database.CreateMasterDataImportBatch(ctx, failedBatch, mutation); err != nil {
		t.Fatal(err)
	}
	_, err = database.ConfirmMasterDataImportBatch(ctx, masterdataimport.ConfirmInput{
		BatchID: failedBatch.ID, Kind: failedBatch.Kind, ActorID: actorID, ExpectedVersion: 1,
		IdempotencyKey: "master-data-import-failed-" + unique, Metadata: mutation,
	})
	if !errors.Is(err, masterdata.ErrDuplicateName) {
		t.Fatalf("duplicate batch confirm error = %v", err)
	}
	var partialCount int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM faculties WHERE id=$1`, firstID).Scan(&partialCount); err != nil {
		t.Fatal(err)
	}
	if partialCount != 0 {
		t.Fatalf("first row survived a later insert failure: %d", partialCount)
	}
	preview, err := database.FindMasterDataImportBatch(ctx, failedBatch.Kind, failedBatch.ID, actorID)
	if err != nil || preview.Status != masterdataimport.StatusPreviewReady || preview.Version != 1 || preview.ConfirmedAt != nil {
		t.Fatalf("failed confirmation changed batch state: %+v, %v", preview, err)
	}

	validRecordID := uuid.NewString()
	validName := fmt.Sprintf("imported-faculty-%s", unique)
	validBatch := masterdataimport.Batch{
		ID: uuid.NewString(), Kind: masterdata.KindFaculty, Status: masterdataimport.StatusPreviewReady, TemplateVersion: 1,
		SourceFileID: seedFile(), TotalRows: 1, ValidRows: 1, Version: 1, CreatedBy: actorID, CreatedAt: time.Now().UTC(),
		Rows: []masterdataimport.PreviewRow{{RowNo: 5, Values: map[string]string{"name": validName}, Status: masterdataimport.RowValid, Errors: []masterdataimport.RowError{}, Record: &masterdata.Record{ID: validRecordID, Kind: masterdata.KindFaculty, Name: validName, Enabled: true}}},
	}
	if err := database.CreateMasterDataImportBatch(ctx, validBatch, mutation); err != nil {
		t.Fatal(err)
	}
	input := masterdataimport.ConfirmInput{BatchID: validBatch.ID, Kind: validBatch.Kind, ActorID: actorID, ExpectedVersion: 1,
		IdempotencyKey: "master-data-import-valid-" + unique, Metadata: mutation}
	result, err := database.ConfirmMasterDataImportBatch(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := database.ConfirmMasterDataImportBatch(ctx, input)
	if err != nil || retry != result {
		t.Fatalf("idempotent retry = %+v, %v; first result = %+v", retry, err, result)
	}
	if result.Status != masterdataimport.StatusConfirmed || result.ImportedRows != 1 || result.Version != 2 {
		t.Fatalf("confirm result = %+v", result)
	}
	confirmed, err := database.FindMasterDataImportBatch(ctx, validBatch.Kind, validBatch.ID, actorID)
	if err != nil || confirmed.ConfirmedAt == nil || confirmed.Status != masterdataimport.StatusConfirmed {
		t.Fatalf("confirmed batch timestamp = %+v, %v", confirmed, err)
	}
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM faculties WHERE id=$1`, validRecordID).Scan(&partialCount); err != nil || partialCount != 1 {
		t.Fatalf("committed row count = %d, %v", partialCount, err)
	}
}

func containsDictionaryOption(options []masterdataimport.DictionaryOption, value, label string) bool {
	for _, option := range options {
		if option.Value == value && option.Label == label {
			return true
		}
	}
	return false
}
