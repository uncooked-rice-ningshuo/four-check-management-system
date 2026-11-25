package disciplineconfig

import (
	"errors"
	"strings"
	"testing"
)

func baseConfig() []Group {
	upper1, upper2, upper3, upper4 := 3.0, 5.0, 7.0, 9.0
	return []Group{{Key: "attendance", Name: "到课", Unit: "学时", Terms: []Term{{InspectionTypeID: "a", SourceUnit: "学时", Operator: "+", Factor: 1}}, Tiers: []Tier{
		{Code: "NOTICE", Name: "通报批评", Lower: 1, Upper: &upper1, AuthorityNumber: "示例〔2026〕1号", ClauseReference: "条款一", TeacherConfirmationRequired: false},
		{Code: "WARNING", Name: "警告", Lower: 3, Upper: &upper2, AuthorityNumber: "示例〔2026〕1号", ClauseReference: "条款二", TeacherConfirmationRequired: true},
		{Code: "SERIOUS_WARNING", Name: "严重警告", Lower: 5, Upper: &upper3, AuthorityNumber: "示例〔2026〕1号", ClauseReference: "条款三", TeacherConfirmationRequired: true},
		{Code: "DEMERIT", Name: "记过", Lower: 7, Upper: &upper4, AuthorityNumber: "示例〔2026〕1号", ClauseReference: "条款四", TeacherConfirmationRequired: true},
		{Code: "PROBATION", Name: "留校察看", Lower: 9, AuthorityNumber: "示例〔2026〕1号", ClauseReference: "条款五", TeacherConfirmationRequired: true},
	}}}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]Group)
		valid  bool
	}{
		{"valid", func([]Group) {}, true},
		{"missing authority", func(g []Group) { g[0].Tiers[1].AuthorityNumber = " " }, false},
		{"missing clause", func(g []Group) { g[0].Tiers[4].ClauseReference = "" }, false},
		{"long authority", func(g []Group) { g[0].Tiers[0].AuthorityNumber = strings.Repeat("号", 121) }, false},
		{"unit required", func(g []Group) { g[0].Terms[0].SourceUnit = "" }, false},
		{"negative conversion", func(g []Group) { g[0].Terms[0].Factor = -1 }, false},
		{"gap", func(g []Group) { g[0].Tiers[1].Lower = 4 }, false},
		{"overlap", func(g []Group) { g[0].Tiers[1].Lower = 2 }, false},
		{"unknown tier", func(g []Group) { g[0].Tiers[0].Code = "SOMETHING" }, false},
		{"wrong source unit", func(g []Group) { g[0].Terms[0].SourceUnit = "次" }, false},
		{"finite top required", func(g []Group) { g[0].Tiers[1].Upper = ptr(9) }, false},
		{"fractional tier lower is rejected", func(g []Group) { g[0].Tiers[0].Lower = 1.5 }, false},
		{"upper exceeds database precision", func(g []Group) { g[0].Tiers[0].Upper = ptr(100000000) }, false},
		{"fractional tier upper is rejected", func(g []Group) { g[0].Tiers[0].Upper = ptr(2.5) }, false},
		{"fractional conversion factor is rejected", func(g []Group) { g[0].Terms[0].Factor = 1.5 }, false},
		{"maximum integer factor accepted", func(g []Group) { g[0].Terms[0].Factor = 999999 }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := baseConfig()
			tt.mutate(g)
			err := Validate(g, []InspectionType{{ID: "a", Code: "ATTENDANCE_RATE", Name: "到课", Enabled: true}}, true)
			if (err == nil) != tt.valid {
				t.Fatalf("Validate() err=%v valid=%v", err, tt.valid)
			}
		})
	}
}

func TestValidateAllowsOrderedTierSubsetAndRejectsDuplicate(t *testing.T) {
	g := baseConfig()
	g[0].Tiers = []Tier{{Code: "WARNING", Name: "警告", Lower: 0, Upper: ptr(3), AuthorityNumber: "示例〔2026〕1号", ClauseReference: "条款", TeacherConfirmationRequired: true}, {Code: "PROBATION", Name: "留校察看", Lower: 3, AuthorityNumber: "示例〔2026〕1号", ClauseReference: "条款", TeacherConfirmationRequired: true}}
	if err := Validate(g, []InspectionType{{ID: "a", Code: "ATTENDANCE_RATE", Name: "到课", Enabled: true}}, true); err != nil {
		t.Fatalf("ordered subset should be valid: %v", err)
	}
	g[0].Tiers[1].Code = "WARNING"
	if err := Validate(g, []InspectionType{{ID: "a", Code: "ATTENDANCE_RATE", Name: "到课", Enabled: true}}, true); err == nil {
		t.Fatal("duplicate tier accepted")
	}
}
func ptr(n float64) *float64 { return &n }

func TestValidateRejectsDuplicateSourceAcrossGroups(t *testing.T) {
	g := baseConfig()
	g = append(g, Group{Key: "second", Name: "第二组", Unit: "次", Terms: []Term{{InspectionTypeID: "a", SourceUnit: "次", Operator: "+", Factor: 1}}})
	if Validate(g, []InspectionType{{ID: "a", Code: "ATTENDANCE_RATE", Name: "到课", Enabled: true}}, false) == nil {
		t.Fatal("duplicate inspection type accepted")
	}
}

func TestValidateReturnsFieldIssueAndNormalizesTypeName(t *testing.T) {
	groups := baseConfig()
	groups[0].Unit = ""
	err := ValidateWithIssues(groups, []InspectionType{{ID: "a", Code: "ATTENDANCE_RATE", Name: "权威名称", Enabled: true}}, true)
	var validation *ValidationError
	if !errors.As(err, &validation) || len(validation.Issues) == 0 || validation.Issues[0].Path == "" {
		t.Fatalf("expected field-level validation issue, got %#v", err)
	}
	groups[0].Terms[0].InspectionTypeName = "客户端伪造名称"
	normalized := NormalizeInspectionTypeNames(groups, []InspectionType{{ID: "a", Code: "ATTENDANCE_RATE", Name: "权威名称", Enabled: true}})
	if normalized[0].Terms[0].InspectionTypeName != "权威名称" {
		t.Fatalf("expected authoritative type name, got %q", normalized[0].Terms[0].InspectionTypeName)
	}
}
