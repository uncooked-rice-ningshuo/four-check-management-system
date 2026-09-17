package inspection

import (
	_ "embed"
	"encoding/json"
	"errors"
)

// ErrUnknownInspectionType is the stable validation code for unsupported catalog lookups.
var ErrUnknownInspectionType = errors.New("VALIDATION_FAILED")

// FieldDefinition describes the input contract enforced by ValidateDetails.
type FieldDefinition struct {
	Key              string   `json:"key"`
	Label            string   `json:"label"`
	Kind             string   `json:"kind"`
	Required         bool     `json:"required"`
	MaxLength        int      `json:"maxLength,omitempty"`
	Minimum          *float64 `json:"minimum,omitempty"`
	ExclusiveMinimum bool     `json:"exclusiveMinimum,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty"`
	MinItems         int      `json:"minItems,omitempty"`
	Enum             []string `json:"enum,omitempty"`
	RequiredWhen     string   `json:"requiredWhen,omitempty"`
	ConstValue       *float64 `json:"constValue,omitempty"`
}

// InspectionDefinition separates planned contracts from complete UI actions.
// Generic CRUD API availability is documented separately from unfinished page/import flows.
type InspectionDefinition struct {
	Code             string            `json:"code"`
	Name             string            `json:"name"`
	ExtensionTable   string            `json:"extensionTable"`
	Granularity      string            `json:"granularity"`
	TemplateVersion  int               `json:"templateVersion"`
	TemplateStatus   string            `json:"templateStatus"`
	Fields           []FieldDefinition `json:"fields"`
	DuplicateKey     []string          `json:"duplicateKey"`
	PlannedActions   []string          `json:"plannedActions"`
	AvailableActions []string          `json:"availableActions"`
	Rules            []string          `json:"rules"`
}

//go:embed catalog.json
var inspectionCatalogJSON []byte

// InspectionCatalog decodes fresh copies, including nested slices and numeric bounds.
func InspectionCatalog() ([]InspectionDefinition, error) {
	var definitions []InspectionDefinition
	if err := json.Unmarshal(inspectionCatalogJSON, &definitions); err != nil {
		return nil, errors.New("invalid internal inspection catalog")
	}
	return definitions, nil
}

// LookupInspectionDefinition does not grant permissions or enable planned features.
func LookupInspectionDefinition(code string) (InspectionDefinition, error) {
	definitions, err := InspectionCatalog()
	if err != nil {
		return InspectionDefinition{}, err
	}
	for _, d := range definitions {
		if d.Code == code {
			return d, nil
		}
	}
	return InspectionDefinition{}, ErrUnknownInspectionType
}
