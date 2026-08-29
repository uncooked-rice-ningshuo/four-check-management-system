package audit

import (
	"errors"
	"strings"
	"testing"
)

func TestSanitizeRejectsMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name  string
		entry Entry
		want  error
	}{
		{name: "action", entry: Entry{ResourceType: "account", ResourceID: "1"}, want: ErrActionRequired},
		{name: "resource type", entry: Entry{Action: ActionAccountUpdated, ResourceID: "1"}, want: ErrResourceTypeRequired},
		{name: "resource id", entry: Entry{Action: ActionAccountUpdated, ResourceType: "account"}, want: ErrResourceIDRequired},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := Sanitize(Metadata{}, test.entry)
			if !errors.Is(err, test.want) {
				t.Fatalf("Sanitize() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestSanitizeLimitsRequestMetadata(t *testing.T) {
	metadata, _, err := Sanitize(Metadata{
		RequestID: strings.Repeat("请", 200),
		IP:        strings.Repeat("1", 100),
		UserAgent: strings.Repeat("浏览器", 300),
		Reason:    strings.Repeat("原因", 200),
	}, Entry{Action: ActionAccountUpdated, ResourceType: "account", ResourceID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := len([]rune(metadata.RequestID)); got != 128 {
		t.Fatalf("request ID length = %d, want 128", got)
	}
	if got := len([]rune(metadata.IP)); got != 64 {
		t.Fatalf("IP length = %d, want 64", got)
	}
	if got := len([]rune(metadata.UserAgent)); got != 512 {
		t.Fatalf("User-Agent length = %d, want 512", got)
	}
	if got := len([]rune(metadata.Reason)); got != 255 {
		t.Fatalf("reason length = %d, want 255", got)
	}
}

func TestSanitizeRedactsSensitiveFieldsRecursively(t *testing.T) {
	_, entry, err := Sanitize(Metadata{}, Entry{
		Action:       ActionAccountUpdated,
		ResourceType: "account",
		ResourceID:   "1",
		Before: map[string]any{
			"password": "plain text",
			"profile": map[string]any{
				"accessToken": "access-token",
				"name":        "张三",
			},
		},
		After: []any{map[string]any{
			"cookie":       "session=secret",
			"clientSecret": "credential",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	before := entry.Before.(map[string]any)
	if before["password"] != RedactedValue {
		t.Fatalf("password = %v", before["password"])
	}
	profile := before["profile"].(map[string]any)
	if profile["accessToken"] != RedactedValue || profile["name"] != "张三" {
		t.Fatalf("profile = %#v", profile)
	}
	after := entry.After.([]any)[0].(map[string]any)
	if after["cookie"] != RedactedValue || after["clientSecret"] != RedactedValue {
		t.Fatalf("after = %#v", after)
	}
}
