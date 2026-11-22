package disciplineconfig

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

var ErrInvalidConfig = errors.New("invalid discipline configuration")
var tierOrder = []string{"NOTICE", "WARNING", "SERIOUS_WARNING", "DEMERIT", "PROBATION"}
var tierNames = map[string]string{"NOTICE": "通报批评", "WARNING": "警告", "SERIOUS_WARNING": "严重警告", "DEMERIT": "记过", "PROBATION": "留校察看"}

type TierDefinition struct {
	Code      string
	Name      string
	SortOrder int
}

const SourceUnitCount = "次"
const maxRuleBound = 99999999.9999
const maxConversionFactor = 999999.999999

func sourceUnitForInspectionCode(code string) string {
	if code == "ATTENDANCE_RATE" {
		return "学时"
	}
	return SourceUnitCount
}

const (
	StatusDraft      = "DRAFT"
	StatusPublished  = "PUBLISHED"
	StatusSuperseded = "SUPERSEDED"
)

type Version struct {
	ID              string  `json:"id"`
	Version         int     `json:"version"`
	Status          string  `json:"status"`
	RowVersion      int     `json:"rowVersion"`
	CreatedBy       string  `json:"createdBy"`
	CreatedByName   string  `json:"createdByName"`
	CreatedAt       string  `json:"createdAt"`
	PublishedBy     string  `json:"publishedBy,omitempty"`
	PublishedByName string  `json:"publishedByName,omitempty"`
	PublishedAt     string  `json:"publishedAt,omitempty"`
	Groups          []Group `json:"groups"`
}

type Group struct {
	ViolationName string `json:"violationName,omitempty"`
	Key           string `json:"key"`
	Name          string `json:"name"`
	Unit          string `json:"unit"`
	Terms         []Term `json:"terms"`
	Tiers         []Tier `json:"tiers"`
}

type Term struct {
	InspectionTypeID   string  `json:"inspectionTypeId"`
	InspectionTypeName string  `json:"inspectionTypeName"`
	SourceUnit         string  `json:"sourceUnit"`
	Operator           string  `json:"operator"`
	Factor             float64 `json:"factor"`
}

type Tier struct {
	AuthorityNumber             string   `json:"authorityNumber,omitempty"`
	ClauseReference             string   `json:"clauseReference,omitempty"`
	Code                        string   `json:"code"`
	Name                        string   `json:"name"`
	Lower                       float64  `json:"lower"`
	Upper                       *float64 `json:"upper,omitempty"`
	Clause                      string   `json:"clause"`
	TeacherConfirmationRequired bool     `json:"teacherConfirmationRequired"`
}

type InspectionType struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type ValidationIssue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type ValidationError struct{ Issues []ValidationIssue }

func (e *ValidationError) Error() string { return ErrInvalidConfig.Error() }
func (e *ValidationError) Unwrap() error { return ErrInvalidConfig }

func Validate(groups []Group, types []InspectionType, complete bool) error {
	return ValidateWithIssues(groups, types, complete)
}

func ValidateWithIssues(groups []Group, types []InspectionType, complete bool) error {
	return ValidateWithTierDefinitions(groups, types, defaultTierDefinitions(), complete)
}

func ValidateWithTierDefinitions(groups []Group, types []InspectionType, definitions []TierDefinition, complete bool) error {
	issues := make([]ValidationIssue, 0)
	add := func(path, code, message string) {
		issues = append(issues, ValidationIssue{Path: path, Code: code, Message: message})
	}
	known := map[string]string{}
	units := map[string]string{}
	for _, t := range types {
		if t.ID != "" {
			known[t.ID] = t.Name
			units[t.ID] = sourceUnitForInspectionCode(t.Code)
		}
	}
	seen := map[string]bool{}
	names := map[string]bool{}
	for gi, g := range groups {
		name := strings.TrimSpace(g.Name)
		if strings.TrimSpace(g.Key) == "" || name == "" || names[name] {
			add(path(gi, "name"), "GROUP_NAME_REQUIRED", "累计组名称不能为空且不能重复")
		}
		if len(g.Terms) == 0 {
			add(path(gi, "terms"), "GROUP_MEMBER_REQUIRED", "累计组至少需要一个检查项")
		}
		names[name] = true
		for i, term := range g.Terms {
			if term.InspectionTypeID == "" || seen[term.InspectionTypeID] {
				add(path(gi, "terms["+strconv.Itoa(i)+"].inspectionTypeId"), "INSPECTION_TYPE_DUPLICATE", "检查项缺失或已加入其他累计组")
				continue
			}
			_, ok := known[term.InspectionTypeID]
			if !ok {
				add(path(gi, "terms["+strconv.Itoa(i)+"].inspectionTypeId"), "INSPECTION_TYPE_UNKNOWN", "检查项不存在或已停用")
				continue
			}
			seen[term.InspectionTypeID] = true
			if !validInteger(term.Factor, maxConversionFactor) || term.Factor <= 0 {
				add(path(gi, "terms["+strconv.Itoa(i)+"].factor"), "INVALID_CONVERSION_FACTOR", "换算系数必须为正整数")
			}
			if complete && strings.TrimSpace(g.Unit) == "" {
				add(path(gi, "unit"), "RESULT_UNIT_REQUIRED", "请填写累计组的统一单位")
			}
			if term.SourceUnit != units[term.InspectionTypeID] {
				add(path(gi, "terms["+strconv.Itoa(i)+"].sourceUnit"), "SOURCE_UNIT_INVALID", "来源单位由检查类型决定，请刷新后重试")
			}
			if (term.Operator != "+" && term.Operator != "-") || (i == 0 && term.Operator != "+") {
				add(path(gi, "terms["+strconv.Itoa(i)+"].operator"), "INVALID_OPERATOR", "首项使用加法，其他项请选择加或减")
			}
		}
		if !complete {
			continue
		}
		if len(g.Tiers) == 0 {
			add(path(gi, "tiers"), "TIER_REQUIRED", "至少保留一个处分档位")
			continue
		}
		lastUpper := 0.0
		lastOrder := -1
		seenTierNames := map[string]bool{}
		orderByCode := make(map[string]int, len(definitions))
		for index, definition := range definitions {
			orderByCode[definition.Code] = index
		}
		for i, t := range g.Tiers {
			tierPath := path(gi, "tiers["+strconv.Itoa(i)+"]")
			order := orderByCode[t.Code]
			if _, exists := orderByCode[t.Code]; !exists {
				order = -1
			}
			if order < 0 || order <= lastOrder {
				add(tierPath+".code", "TIER_ORDER_INVALID", "处分档位须来自字典且按档位顺序排列，不能重复")
			}
			if strings.TrimSpace(t.Clause) == "" {
				add(tierPath+".clause", "CLAUSE_REQUIRED", "请填写对应处分依据条款")
			}
			if !validInteger(t.Lower, maxRuleBound) {
				add(tierPath+".lower", "INVALID_TIER_BOUND", "起始值必须为 0 至 99999999 之间的整数")
			}
			if i > 0 && t.Lower != lastUpper {
				add(tierPath+".lower", "TIER_GAP_OR_OVERLAP", "处分区间存在空档或重叠")
			}
			if strings.TrimSpace(t.Name) == "" || seenTierNames[strings.TrimSpace(t.Name)] {
				add(tierPath+".name", "TIER_NAME_INVALID", "处分名称不能为空且同一累计项内不能重复")
			}
			seenTierNames[strings.TrimSpace(t.Name)] = true
			if t.Upper != nil && (!validInteger(*t.Upper, maxRuleBound) || *t.Upper <= t.Lower) {
				add(tierPath+".upper", "INVALID_TIER_BOUND", "结束值必须为大于起始值且不超过 99999999 的整数")
			}
			if t.Upper != nil {
				lastUpper = *t.Upper
			} else if i != len(g.Tiers)-1 {
				add(tierPath+".upper", "TIER_UPPER_REQUIRED", "仅最高档位的结束值可以留空")
			}
			lastOrder = order
		}
		if len(g.Tiers) > 0 && g.Tiers[len(g.Tiers)-1].Upper != nil {
			add(path(gi, "tiers["+strconv.Itoa(len(g.Tiers)-1)+"].upper"), "TOP_TIER_MUST_BE_OPEN", "最高档位的结束值应留空")
		}
	}
	if complete {
		for _, t := range types {
			if t.Enabled && !seen[t.ID] {
				add("groups", "INSPECTION_TYPE_UNASSIGNED", "每个启用的检查项都必须配置累计组")
			}
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

func defaultTierDefinitions() []TierDefinition {
	result := make([]TierDefinition, 0, len(tierOrder))
	for i, code := range tierOrder {
		result = append(result, TierDefinition{Code: code, Name: tierNames[code], SortOrder: (i + 1) * 10})
	}
	return result
}

func finite(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) }
func validInteger(value, max float64) bool {
	return finite(value) && value >= 0 && value <= max && math.Trunc(value) == value
}
func path(group int, field string) string { return "groups[" + strconv.Itoa(group) + "]." + field }

func NormalizeInspectionTypeNames(groups []Group, types []InspectionType) []Group {
	return NormalizeWithTierDefinitions(groups, types, defaultTierDefinitions())
}

func NormalizeWithTierDefinitions(groups []Group, types []InspectionType, definitions []TierDefinition) []Group {
	names := make(map[string]string, len(types))
	for _, item := range types {
		names[item.ID] = item.Name
	}
	normalized := make([]Group, len(groups))
	copy(normalized, groups)
	for i := range normalized {
		normalized[i].Terms = append([]Term(nil), groups[i].Terms...)
		for j := range normalized[i].Terms {
			for _, item := range types {
				if item.ID == normalized[i].Terms[j].InspectionTypeID {
					normalized[i].Terms[j].SourceUnit = sourceUnitForInspectionCode(item.Code)
					break
				}
			}
			if name, ok := names[normalized[i].Terms[j].InspectionTypeID]; ok {
				normalized[i].Terms[j].InspectionTypeName = name
			}
		}
		normalized[i].Tiers = append([]Tier(nil), groups[i].Tiers...)
	}
	return normalized
}
