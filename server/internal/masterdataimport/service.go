package masterdataimport

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/masterdata"
	"github.com/google/uuid"
)

var yearPattern = regexp.MustCompile(`^[0-9]{4}$`)
var nonNegativeIntegerPattern = regexp.MustCompile(`^[0-9]+$`)

type Service struct {
	repository Repository
	catalog    Catalog
	maxBytes   int64
}

func NewService(repository Repository, catalog Catalog, maxBytes int64) *Service {
	return &Service{repository: repository, catalog: catalog, maxBytes: maxBytes}
}

func (s *Service) Preview(ctx context.Context, kind, fileID, filename, actorID string, content []byte, metadata audit.Metadata) (Batch, error) {
	spec, ok := SpecFor(kind)
	if !ok {
		return Batch{}, ErrInvalidKind
	}
	if s.repository == nil || s.catalog == nil || s.maxBytes <= 0 || strings.TrimSpace(fileID) == "" || strings.TrimSpace(actorID) == "" {
		return Batch{}, ErrInvalidBatch
	}
	parser, err := NewParser(kind, s.maxBytes)
	if err != nil {
		return Batch{}, err
	}
	workbook, err := parser.Parse(filename, content)
	if err != nil {
		return Batch{}, err
	}
	if len(workbook.Rows) == 0 {
		return Batch{}, ErrEmptyWorkbook
	}
	rows, err := s.validateRows(ctx, spec, workbook.Rows)
	if err != nil {
		return Batch{}, err
	}
	batch := Batch{
		ID: uuid.NewString(), Kind: kind, Status: StatusPreviewReady, TemplateVersion: spec.TemplateVersion,
		Fields:       spec.Fields,
		SourceFileID: fileID, TotalRows: len(rows), Version: 1, Rows: rows,
		CreatedBy: actorID, CreatedAt: time.Now().UTC(),
	}
	for _, row := range rows {
		if row.Status == RowValid {
			batch.ValidRows++
		} else {
			batch.InvalidRows++
		}
	}
	if batch.InvalidRows > 0 {
		batch.Status = StatusPreviewHasErrors
	}
	metadata.ActorID = actorID
	if err := s.repository.CreateMasterDataImportBatch(ctx, batch, metadata); err != nil {
		return Batch{}, err
	}
	return batch, nil
}

func (s *Service) GetBatch(ctx context.Context, kind, id, actorID string) (Batch, error) {
	if _, ok := SpecFor(kind); !ok {
		return Batch{}, ErrInvalidKind
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(actorID) == "" {
		return Batch{}, ErrBatchNotFound
	}
	batch, err := s.repository.FindMasterDataImportBatch(ctx, kind, id, actorID)
	if err != nil {
		return Batch{}, err
	}
	spec, _ := SpecFor(kind)
	batch.Fields = spec.Fields
	return batch, nil
}

func (s *Service) Confirm(ctx context.Context, input ConfirmInput) (ConfirmResult, error) {
	if _, ok := SpecFor(input.Kind); !ok {
		return ConfirmResult{}, ErrInvalidKind
	}
	input.BatchID = strings.TrimSpace(input.BatchID)
	input.ActorID = strings.TrimSpace(input.ActorID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.BatchID == "" || input.ActorID == "" || input.ExpectedVersion < 1 || len(input.IdempotencyKey) < 16 || len(input.IdempotencyKey) > 128 {
		return ConfirmResult{}, ErrInvalidIdempotencyKey
	}
	input.Metadata.ActorID = input.ActorID
	return s.repository.ConfirmMasterDataImportBatch(ctx, input)
}

func (s *Service) validateRows(ctx context.Context, spec TemplateSpec, parsed []importjob.ParsedRow) ([]PreviewRow, error) {
	rows := make([]PreviewRow, len(parsed))
	for index, row := range parsed {
		rows[index] = PreviewRow{RowNo: row.RowNo, Values: cloneValues(row.Values), Status: RowValid, Errors: make([]RowError, 0)}
	}
	addError := func(index int, field, message string) {
		rows[index].Errors = append(rows[index].Errors, RowError{Field: field, Message: message})
	}

	records := make([]masterdata.Record, len(parsed))
	validPrimitive := make([]bool, len(parsed))
	for index, parsedRow := range parsed {
		record, ok := primitiveRecord(spec, parsedRow.Values, func(field, message string) { addError(index, field, message) })
		records[index], validPrimitive[index] = record, ok
	}

	lookups := newReferenceLookups()
	if err := collectReferenceLookups(ctx, s.catalog, spec.Kind, parsed, validPrimitive, lookups); err != nil {
		return nil, err
	}
	for index, parsedRow := range parsed {
		if !validPrimitive[index] {
			continue
		}
		resolveRecordReferences(spec.Kind, parsedRow.Values, &records[index], lookups, func(field, message string) {
			addError(index, field, message)
		})
	}
	if err := validateDictionaries(ctx, s.catalog, spec.Kind, parsed, records, validPrimitive, addError); err != nil {
		return nil, err
	}
	markInFileDuplicates(spec.Kind, parsed, records, validPrimitive, addError)
	if err := markExistingDuplicates(ctx, s.catalog, spec.Kind, parsed, records, validPrimitive, addError); err != nil {
		return nil, err
	}
	for index := range rows {
		if len(rows[index].Errors) > 0 {
			rows[index].Status = RowInvalid
			continue
		}
		record := records[index]
		record.ID = uuid.NewString()
		record.Enabled = true
		rows[index].Record = &record
	}
	return rows, nil
}

func primitiveRecord(spec TemplateSpec, values map[string]string, addError func(string, string)) (masterdata.Record, bool) {
	trimmed := make(map[string]string, len(values))
	for key, value := range values {
		trimmed[key] = strings.TrimSpace(value)
	}
	valid := true
	for _, field := range spec.Fields {
		value := trimmed[field.Code]
		if field.Required && value == "" {
			addError(field.Code, "必填")
			valid = false
			continue
		}
		if value != "" && field.MaxRunes > 0 && utf8.RuneCountInString(value) > field.MaxRunes {
			addError(field.Code, fmt.Sprintf("最多 %d 个字符", field.MaxRunes))
			valid = false
		}
	}

	record := masterdata.Record{Kind: spec.Kind}
	if spec.Kind == masterdata.KindGrade {
		year := trimmed["year"]
		if year != "" && !yearPattern.MatchString(year) {
			addError("year", "须为 4 位 ASCII 数字，例如 2026")
			valid = false
		}
		if year != "" {
			record.Name = year + "级"
		}
	} else {
		switch spec.Kind {
		case masterdata.KindDormitory:
			record.RoomNo = trimmed["room_no"]
			record.Name = record.RoomNo
		case masterdata.KindClassroom:
			record.RoomNo = trimmed["room_no"]
			record.Name = record.RoomNo
		default:
			record.Name = trimmed["name"]
		}
	}
	record.Code = strings.ToUpper(trimmed["employee_no"])
	if record.Code == "" {
		record.Code = strings.ToUpper(trimmed["student_no"])
	}
	record.Gender = trimmed["gender"]
	if record.Gender == "" && (spec.Kind == masterdata.KindStaff || spec.Kind == masterdata.KindStudent) {
		record.Gender = "UNKNOWN"
	}
	record.Phone = trimmed["phone"]
	record.EnrollmentStatus = trimmed["enrollment_status"]
	if spec.Kind == masterdata.KindStudent && record.EnrollmentStatus == "" {
		record.EnrollmentStatus = "ENROLLED"
	}
	if spec.Kind == masterdata.KindClass {
		year := trimmed["grade_year"]
		if year != "" && !yearPattern.MatchString(year) {
			addError("grade_year", "须为 4 位 ASCII 数字，例如 2026")
			valid = false
		}
		if year != "" {
			record.Grade = year + "级"
		}
		if sizeText := trimmed["enrolled_size"]; sizeText != "" {
			if !nonNegativeIntegerPattern.MatchString(sizeText) {
				addError("enrolled_size", "须为不带符号的十进制非负整数")
				valid = false
			} else if size, err := strconv.ParseInt(sizeText, 10, 32); err != nil {
				addError("enrolled_size", "超出 32 位非负整数范围 0–2147483647")
				valid = false
			} else {
				record.EnrolledSize = int(size)
			}
		}
	}
	if spec.Kind == masterdata.KindStudent {
		year := trimmed["grade_year"]
		if year != "" && !yearPattern.MatchString(year) {
			addError("grade_year", "须为 4 位 ASCII 数字，例如 2026")
			valid = false
		}
		if year != "" {
			record.Grade = year + "级"
		}
	}
	return record, valid
}

type referenceLookups struct {
	byName map[string][]masterdata.Record
	byCode map[string][]masterdata.Record
}

func newReferenceLookups() referenceLookups {
	return referenceLookups{byName: make(map[string][]masterdata.Record), byCode: make(map[string][]masterdata.Record)}
}

func lookupKey(kind, value string) string { return kind + "\x00" + value }

func addNameKeys(target map[string]LookupKey, values ...string) {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			target[value] = LookupKey{Name: value}
		}
	}
}

func addCodeKeys(target map[string]LookupKey, values ...string) {
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value != "" {
			target[value] = LookupKey{Code: value}
		}
	}
}

func collectReferenceLookups(ctx context.Context, catalog Catalog, targetKind string, parsed []importjob.ParsedRow, valid []bool, result referenceLookups) error {
	namesByKind := map[string]map[string]LookupKey{}
	codesByKind := map[string]map[string]LookupKey{}
	addNames := func(kind string, values ...string) {
		if namesByKind[kind] == nil {
			namesByKind[kind] = map[string]LookupKey{}
		}
		addNameKeys(namesByKind[kind], values...)
	}
	addCodes := func(kind string, values ...string) {
		if codesByKind[kind] == nil {
			codesByKind[kind] = map[string]LookupKey{}
		}
		addCodeKeys(codesByKind[kind], values...)
	}
	for index, row := range parsed {
		if !valid[index] {
			continue
		}
		values := row.Values
		for _, field := range []string{"faculty_name"} {
			addNames(masterdata.KindFaculty, values[field])
		}
		for _, field := range []string{"grade_year", "year"} {
			year := strings.TrimSpace(values[field])
			if yearPattern.MatchString(year) {
				addNames(masterdata.KindGrade, year+"级")
			}
		}
		addNames(masterdata.KindDormitoryArea, values["area_name"], values["dormitory_area_name"])
		addNames(masterdata.KindTeachingBuilding, values["building_name"])
		addNames(masterdata.KindClass, values["class_name"])
		addCodes(masterdata.KindStaff, values["head_teacher_employee_no"])
	}
	for kind, keys := range namesByKind {
		if len(keys) == 0 {
			continue
		}
		found, err := catalog.FindMasterDataImportRecords(ctx, kind, valuesOfKeys(keys), false)
		if err != nil {
			return err
		}
		for _, record := range found {
			result.byName[lookupKey(kind, record.Name)] = append(result.byName[lookupKey(kind, record.Name)], record)
		}
	}
	for kind, keys := range codesByKind {
		if len(keys) == 0 {
			continue
		}
		found, err := catalog.FindMasterDataImportRecords(ctx, kind, valuesOfKeys(keys), false)
		if err != nil {
			return err
		}
		for _, record := range found {
			result.byCode[lookupKey(kind, strings.ToUpper(record.Code))] = append(result.byCode[lookupKey(kind, strings.ToUpper(record.Code))], record)
		}
	}
	if targetKind == masterdata.KindMajor || targetKind == masterdata.KindClass || targetKind == masterdata.KindStudent {
		majorNames := map[string]LookupKey{}
		for index, row := range parsed {
			if valid[index] {
				addNameKeys(majorNames, row.Values["major_name"])
			}
		}
		if len(majorNames) > 0 {
			found, err := catalog.FindMasterDataImportRecords(ctx, masterdata.KindMajor, valuesOfKeys(majorNames), false)
			if err != nil {
				return err
			}
			for _, record := range found {
				result.byName[lookupKey(masterdata.KindMajor, record.Name)] = append(result.byName[lookupKey(masterdata.KindMajor, record.Name)], record)
			}
		}
	}
	if targetKind == masterdata.KindDormitoryBuilding || targetKind == masterdata.KindDormitory || targetKind == masterdata.KindStudent {
		buildingNames := map[string]LookupKey{}
		for index, row := range parsed {
			if valid[index] {
				addNameKeys(buildingNames, row.Values["building_name"], row.Values["dormitory_building_name"])
			}
		}
		if len(buildingNames) > 0 {
			found, err := catalog.FindMasterDataImportRecords(ctx, masterdata.KindDormitoryBuilding, valuesOfKeys(buildingNames), false)
			if err != nil {
				return err
			}
			for _, record := range found {
				result.byName[lookupKey(masterdata.KindDormitoryBuilding, record.Name)] = append(result.byName[lookupKey(masterdata.KindDormitoryBuilding, record.Name)], record)
			}
		}
	}
	if targetKind == masterdata.KindStudent {
		dormitoryKeys := make(map[string]LookupKey)
		for index, row := range parsed {
			if !valid[index] || strings.TrimSpace(row.Values["dormitory_area_name"]) == "" || strings.TrimSpace(row.Values["dormitory_building_name"]) == "" || strings.TrimSpace(row.Values["room_no"]) == "" {
				continue
			}
			areaMatches := result.byName[lookupKey(masterdata.KindDormitoryArea, strings.TrimSpace(row.Values["dormitory_area_name"]))]
			if len(areaMatches) != 1 {
				continue
			}
			buildingMatches := filterReference(result.byName[lookupKey(masterdata.KindDormitoryBuilding, strings.TrimSpace(row.Values["dormitory_building_name"]))], func(item masterdata.Record) bool {
				return item.DormitoryAreaID == areaMatches[0].ID
			})
			if len(buildingMatches) != 1 {
				continue
			}
			roomNo := strings.TrimSpace(row.Values["room_no"])
			dormitoryKeys[buildingMatches[0].ID+"\x00"+roomNo] = LookupKey{ParentID: buildingMatches[0].ID, RoomNo: roomNo}
		}
		if len(dormitoryKeys) > 0 {
			found, err := catalog.FindMasterDataImportRecords(ctx, masterdata.KindDormitory, valuesOfKeys(dormitoryKeys), false)
			if err != nil {
				return err
			}
			for _, record := range found {
				key := record.DormitoryBuildingID + "\x00" + record.RoomNo
				result.byName[lookupKey(masterdata.KindDormitory, key)] = append(result.byName[lookupKey(masterdata.KindDormitory, key)], record)
			}
		}
	}
	if targetKind == masterdata.KindClassroom {
		// Teaching buildings are already loaded as direct name references.
	}
	return nil
}

func valuesOfKeys(values map[string]LookupKey) []LookupKey {
	result := make([]LookupKey, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func resolveRecordReferences(kind string, values map[string]string, record *masterdata.Record, lookups referenceLookups, addError func(string, string)) {
	resolveName := func(field, refKind, name string) (masterdata.Record, bool) {
		name = strings.TrimSpace(name)
		if name == "" {
			return masterdata.Record{}, false
		}
		matches := lookups.byName[lookupKey(refKind, name)]
		if len(matches) == 0 {
			addError(field, "未找到同名的已启用基础数据")
			return masterdata.Record{}, false
		}
		if len(matches) > 1 {
			addError(field, "名称对应多条已启用基础数据，请补充所属院系或宿舍区")
			return masterdata.Record{}, false
		}
		return matches[0], true
	}
	resolveCode := func(field, refKind, code string) (masterdata.Record, bool) {
		code = strings.ToUpper(strings.TrimSpace(code))
		if code == "" {
			return masterdata.Record{}, false
		}
		matches := lookups.byCode[lookupKey(refKind, code)]
		if len(matches) == 0 {
			addError(field, "未找到该工号对应的已启用教职工")
			return masterdata.Record{}, false
		}
		if len(matches) > 1 {
			addError(field, "工号对应多条已启用教职工")
			return masterdata.Record{}, false
		}
		return matches[0], true
	}
	resolveFaculty := func(field, name string) {
		if faculty, ok := resolveName(field, masterdata.KindFaculty, name); ok {
			record.FacultyID = faculty.ID
		}
	}
	resolveGrade := func(field, grade string) {
		if grade == "" {
			return
		}
		if item, ok := resolveName(field, masterdata.KindGrade, grade); ok {
			record.GradeID = item.ID
			record.Grade = item.Name
		}
	}
	resolveMajor := func(field, name string) {
		if name == "" {
			return
		}
		matches := lookups.byName[lookupKey(masterdata.KindMajor, strings.TrimSpace(name))]
		if record.FacultyID != "" {
			filtered := make([]masterdata.Record, 0, len(matches))
			for _, item := range matches {
				if item.FacultyID == record.FacultyID {
					filtered = append(filtered, item)
				}
			}
			matches = filtered
		}
		if len(matches) == 0 {
			addError(field, "未找到与所属院系匹配的已启用专业")
			return
		}
		if len(matches) > 1 {
			addError(field, "同名专业跨院系重复，请填写所属院系")
			return
		}
		major := matches[0]
		record.MajorID = major.ID
		if record.FacultyID == "" {
			record.FacultyID = major.FacultyID
		} else if record.FacultyID != major.FacultyID {
			addError(field, "专业与所属院系不匹配")
		}
	}

	switch kind {
	case masterdata.KindMajor:
		resolveFaculty("faculty_name", values["faculty_name"])
	case masterdata.KindStaff:
		resolveFaculty("faculty_name", values["faculty_name"])
	case masterdata.KindClass:
		resolveGrade("grade_year", record.Grade)
		resolveFaculty("faculty_name", values["faculty_name"])
		resolveMajor("major_name", values["major_name"])
		if teacher, ok := resolveCode("head_teacher_employee_no", masterdata.KindStaff, values["head_teacher_employee_no"]); ok {
			record.HeadTeacherID = teacher.ID
		}
	case masterdata.KindDormitoryBuilding:
		resolveDormitoryArea("area_name", values["area_name"], record, lookups, addError)
	case masterdata.KindDormitory:
		area, ok := resolveName("area_name", masterdata.KindDormitoryArea, values["area_name"])
		if !ok {
			break
		}
		buildings := filterReference(lookups.byName[lookupKey(masterdata.KindDormitoryBuilding, strings.TrimSpace(values["building_name"]))], func(item masterdata.Record) bool {
			return item.DormitoryAreaID == area.ID
		})
		if len(buildings) != 1 {
			addError("building_name", referenceError(len(buildings), "宿舍区内未找到唯一的已启用宿舍楼"))
			break
		}
		record.DormitoryAreaID = area.ID
		record.DormitoryBuildingID = buildings[0].ID
		// Dormitory records are resolved by their parent ID plus room number below.
		if found := lookups.byName[lookupKey(masterdata.KindDormitory, buildings[0].ID+"\x00"+record.RoomNo)]; len(found) > 0 {
			addError("room_no", "该宿舍已存在")
		}
	case masterdata.KindClassroom:
		if building, ok := resolveName("building_name", masterdata.KindTeachingBuilding, values["building_name"]); ok {
			record.TeachingBuildingID = building.ID
		}
	case masterdata.KindStudent:
		resolveGrade("grade_year", record.Grade)
		resolveFaculty("faculty_name", values["faculty_name"])
		resolveMajor("major_name", values["major_name"])
		if values["class_name"] != "" {
			if class, ok := resolveName("class_name", masterdata.KindClass, values["class_name"]); ok {
				record.ClassID = class.ID
				if record.GradeID != "" && class.GradeID != record.GradeID {
					addError("class_name", "班级所属年级与本行入学年份不一致")
				}
				if record.FacultyID != "" && class.FacultyID != "" && record.FacultyID != class.FacultyID {
					addError("class_name", "班级与所属院系不匹配")
				}
				if record.MajorID != "" && class.MajorID != "" && record.MajorID != class.MajorID {
					addError("class_name", "班级与专业不匹配")
				}
			}
		}
		areaName, buildingName, roomNo := values["dormitory_area_name"], values["dormitory_building_name"], values["room_no"]
		present := 0
		for _, value := range []string{areaName, buildingName, roomNo} {
			if strings.TrimSpace(value) != "" {
				present++
			}
		}
		if present != 0 && present != 3 {
			addError("dormitory_area_name", "宿舍区、宿舍楼和宿舍号须同时填写或同时留空")
			break
		}
		if present == 3 {
			area, ok := resolveName("dormitory_area_name", masterdata.KindDormitoryArea, areaName)
			if !ok {
				break
			}
			buildings := filterReference(lookups.byName[lookupKey(masterdata.KindDormitoryBuilding, strings.TrimSpace(buildingName))], func(item masterdata.Record) bool {
				return item.DormitoryAreaID == area.ID
			})
			if len(buildings) != 1 {
				addError("dormitory_building_name", referenceError(len(buildings), "宿舍区内未找到唯一的已启用宿舍楼"))
				break
			}
			key := buildings[0].ID + "\x00" + strings.TrimSpace(roomNo)
			if found := lookups.byName[lookupKey(masterdata.KindDormitory, key)]; len(found) == 0 {
				addError("room_no", "未找到与宿舍区、宿舍楼和宿舍号匹配的已启用宿舍")
			} else if len(found) > 1 {
				addError("room_no", "宿舍定位对应多条已启用基础数据")
			} else {
				record.DormitoryID = found[0].ID
				record.DormitoryBuildingID = buildings[0].ID
				record.DormitoryAreaID = area.ID
			}
		}
	}
}

func resolveDormitoryArea(field, name string, record *masterdata.Record, lookups referenceLookups, addError func(string, string)) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	matches := lookups.byName[lookupKey(masterdata.KindDormitoryArea, name)]
	if len(matches) == 0 {
		addError(field, "未找到同名的已启用宿舍区")
		return
	}
	if len(matches) > 1 {
		addError(field, "宿舍区名称对应多条已启用基础数据")
		return
	}
	record.DormitoryAreaID = matches[0].ID
}

func filterReference(values []masterdata.Record, include func(masterdata.Record) bool) []masterdata.Record {
	result := make([]masterdata.Record, 0, len(values))
	for _, value := range values {
		if include(value) {
			result = append(result, value)
		}
	}
	return result
}

func referenceError(count int, noMatch string) string {
	if count > 1 {
		return "名称对应多条已启用基础数据"
	}
	return noMatch
}

func validateDictionaries(ctx context.Context, catalog Catalog, kind string, parsed []importjob.ParsedRow, records []masterdata.Record, valid []bool, addError func(int, string, string)) error {
	if kind != masterdata.KindStaff && kind != masterdata.KindStudent {
		return nil
	}
	genderOptions, err := catalog.EnabledDictionaryOptions(ctx, "GENDER")
	if err != nil {
		return err
	}
	for index := range parsed {
		if valid[index] {
			value, ok, ambiguous := resolveDictionaryOption(records[index].Gender, genderOptions)
			if ambiguous {
				addError(index, "gender", "输入内容匹配多条已启用 GENDER 字典项，请填写能唯一匹配的选项值")
			} else if !ok {
				addError(index, "gender", "不是当前启用的 GENDER 字典选项值或显示名称")
			} else {
				records[index].Gender = value
			}
		}
	}
	if kind == masterdata.KindStudent {
		statusOptions, err := catalog.EnabledDictionaryOptions(ctx, "ENROLLMENT_STATUS")
		if err != nil {
			return err
		}
		for index := range parsed {
			if valid[index] {
				value, ok, ambiguous := resolveDictionaryOption(records[index].EnrollmentStatus, statusOptions)
				if ambiguous {
					addError(index, "enrollment_status", "输入内容匹配多条已启用学籍状态字典项，请填写能唯一匹配的选项值")
				} else if !ok {
					addError(index, "enrollment_status", "不是当前启用的 ENROLLMENT_STATUS 字典选项值或显示名称")
				} else {
					records[index].EnrollmentStatus = value
				}
			}
		}
	}
	return nil
}

func resolveDictionaryOption(input string, options []DictionaryOption) (value string, ok bool, ambiguous bool) {
	input = strings.TrimSpace(input)
	for _, option := range options {
		if input == strings.TrimSpace(option.Value) {
			return option.Value, true, false
		}
	}
	for _, option := range options {
		if strings.EqualFold(input, strings.TrimSpace(option.Value)) {
			if ok {
				return "", false, true
			}
			value, ok = option.Value, true
		}
	}
	if ok {
		return value, true, false
	}
	for _, option := range options {
		if strings.EqualFold(input, strings.TrimSpace(option.Label)) {
			if ok {
				return "", false, true
			}
			value, ok = option.Value, true
		}
	}
	return value, ok, false
}

func markInFileDuplicates(kind string, parsed []importjob.ParsedRow, records []masterdata.Record, valid []bool, addError func(int, string, string)) {
	seen := map[string][]int{}
	for index, row := range parsed {
		key := naturalKey(kind, row.Values, records[index])
		if key != "" {
			seen[key] = append(seen[key], index)
		}
	}
	for _, indexes := range seen {
		if len(indexes) < 2 {
			continue
		}
		for _, index := range indexes {
			addError(index, duplicateField(kind), "本文件中存在重复记录")
		}
	}
}

func markExistingDuplicates(ctx context.Context, catalog Catalog, kind string, parsed []importjob.ParsedRow, records []masterdata.Record, valid []bool, addError func(int, string, string)) error {
	keysByIndex := make(map[int]LookupKey)
	keys := make([]LookupKey, 0, len(records))
	for index, record := range records {
		if !valid[index] || recordHasErrors(parsed[index], kind) {
			continue
		}
		key, ok := recordLookupKey(kind, parsed[index].Values, record)
		if !ok {
			continue
		}
		keysByIndex[index] = key
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil
	}
	found, err := catalog.FindMasterDataImportRecords(ctx, kind, keys, true)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return nil
	}
	for index, key := range keysByIndex {
		if matchesKey(kind, found, key) {
			addError(index, duplicateField(kind), "与已有基础数据重复，停用记录也不能重复新增")
		}
	}
	return nil
}

func recordHasErrors(parsed importjob.ParsedRow, kind string) bool {
	// Required/reference shape is checked by the per-row mapper. Natural-key fields
	// are checked here again before querying because invalid rows must never enter the write set.
	for _, field := range append(requiredFields(kind), naturalFields(kind)...) {
		if strings.TrimSpace(parsed.Values[field]) == "" {
			return true
		}
	}
	return false
}

func requiredFields(kind string) []string {
	spec, _ := SpecFor(kind)
	var fields []string
	for _, field := range spec.Fields {
		if field.Required {
			fields = append(fields, field.Code)
		}
	}
	return fields
}

func naturalFields(kind string) []string {
	switch kind {
	case masterdata.KindDormitory, masterdata.KindClassroom:
		return []string{"room_no"}
	case masterdata.KindStaff:
		return []string{"employee_no"}
	case masterdata.KindStudent:
		return []string{"student_no"}
	default:
		return []string{"name"}
	}
}

func naturalKey(kind string, values map[string]string, record masterdata.Record) string {
	parts := []string{kind}
	switch kind {
	case masterdata.KindMajor:
		parts = append(parts, strings.TrimSpace(values["faculty_name"]), strings.TrimSpace(values["name"]))
	case masterdata.KindDormitoryBuilding:
		parts = append(parts, strings.TrimSpace(values["area_name"]), strings.TrimSpace(values["name"]))
	case masterdata.KindDormitory:
		parts = append(parts, strings.TrimSpace(values["area_name"]), strings.TrimSpace(values["building_name"]), strings.TrimSpace(values["room_no"]))
	case masterdata.KindClassroom:
		parts = append(parts, strings.TrimSpace(values["building_name"]), strings.TrimSpace(values["room_no"]))
	case masterdata.KindStaff:
		parts = append(parts, strings.ToUpper(strings.TrimSpace(values["employee_no"])))
	case masterdata.KindStudent:
		parts = append(parts, strings.ToUpper(strings.TrimSpace(values["student_no"])))
	case masterdata.KindGrade:
		parts = append(parts, record.Name)
	default:
		parts = append(parts, strings.TrimSpace(values["name"]))
	}
	for _, part := range parts {
		if part == "" {
			return ""
		}
	}
	return strings.Join(parts, "\x00")
}

func recordLookupKey(kind string, values map[string]string, record masterdata.Record) (LookupKey, bool) {
	switch kind {
	case masterdata.KindFaculty, masterdata.KindGrade, masterdata.KindClass, masterdata.KindDormitoryArea, masterdata.KindTeachingBuilding:
		return LookupKey{Name: record.Name}, record.Name != ""
	case masterdata.KindMajor:
		return LookupKey{ParentID: record.FacultyID, Name: record.Name}, record.FacultyID != "" && record.Name != ""
	case masterdata.KindStaff:
		return LookupKey{Code: record.Code}, record.Code != ""
	case masterdata.KindDormitoryBuilding:
		return LookupKey{ParentID: record.DormitoryAreaID, Name: record.Name}, record.DormitoryAreaID != "" && record.Name != ""
	case masterdata.KindDormitory:
		return LookupKey{ParentID: record.DormitoryBuildingID, RoomNo: record.RoomNo}, record.DormitoryBuildingID != "" && record.RoomNo != ""
	case masterdata.KindClassroom:
		return LookupKey{ParentID: record.TeachingBuildingID, RoomNo: record.RoomNo}, record.TeachingBuildingID != "" && record.RoomNo != ""
	case masterdata.KindStudent:
		return LookupKey{Code: record.Code}, record.Code != ""
	default:
		return LookupKey{}, false
	}
}

func duplicateField(kind string) string {
	switch kind {
	case masterdata.KindStaff:
		return "employee_no"
	case masterdata.KindStudent:
		return "student_no"
	case masterdata.KindDormitory, masterdata.KindClassroom:
		return "room_no"
	default:
		return "name"
	}
}

func matchesKey(kind string, records []masterdata.Record, key LookupKey) bool {
	for _, record := range records {
		switch kind {
		case masterdata.KindMajor:
			if record.Name == key.Name && (key.ParentID == "" || record.FacultyID == key.ParentID) {
				return true
			}
		case masterdata.KindDormitoryBuilding:
			if record.Name == key.Name && (key.ParentID == "" || record.DormitoryAreaID == key.ParentID) {
				return true
			}
		case masterdata.KindDormitory:
			if key.ParentID == "" && key.RoomNo == "" && record.Name == key.Name {
				return true
			}
			if record.DormitoryBuildingID == key.ParentID && record.RoomNo == key.RoomNo {
				return true
			}
		case masterdata.KindClassroom:
			if key.ParentID == "" && key.RoomNo == "" && record.Name == key.Name {
				return true
			}
			if record.TeachingBuildingID == key.ParentID && record.RoomNo == key.RoomNo {
				return true
			}
		case masterdata.KindStaff:
			if strings.EqualFold(record.Code, key.Code) {
				return true
			}
		case masterdata.KindStudent:
			if strings.EqualFold(record.Code, key.Code) {
				return true
			}
		default:
			if record.Name == key.Name {
				return true
			}
		}
	}
	return false
}

func cloneValues(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = strings.TrimSpace(value)
	}
	return result
}
