package audit

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const RedactedValue = "[REDACTED]"

// Sanitize 校验审计标识、限制请求元数据长度，并递归移除敏感字段值。
func Sanitize(metadata Metadata, entry Entry) (Metadata, Entry, error) {
	if entry.Action == "" {
		return Metadata{}, Entry{}, ErrActionRequired
	}
	if strings.TrimSpace(entry.ResourceType) == "" {
		return Metadata{}, Entry{}, ErrResourceTypeRequired
	}
	if strings.TrimSpace(entry.ResourceID) == "" {
		return Metadata{}, Entry{}, ErrResourceIDRequired
	}

	metadata.RequestID = truncate(metadata.RequestID, 128)
	metadata.IP = truncate(metadata.IP, 64)
	metadata.UserAgent = truncate(metadata.UserAgent, 512)
	metadata.Reason = truncate(metadata.Reason, 255)

	before, err := sanitizeJSON(entry.Before)
	if err != nil {
		return Metadata{}, Entry{}, fmt.Errorf("sanitize audit before value: %w", err)
	}
	after, err := sanitizeJSON(entry.After)
	if err != nil {
		return Metadata{}, Entry{}, fmt.Errorf("sanitize audit after value: %w", err)
	}
	entry.Before = before
	entry.After = after
	return metadata, entry, nil
}

func truncate(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func sanitizeJSON(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil, err
	}
	return redact(normalized), nil
}

func redact(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if isSensitiveKey(key) {
				result[key] = RedactedValue
				continue
			}
			result[key] = redact(child)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = redact(child)
		}
		return result
	default:
		return typed
	}
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(key)
	for _, fragment := range []string{"password", "token", "cookie", "secret"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}
