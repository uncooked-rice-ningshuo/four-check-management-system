package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRejectsInvalidRecoveryArgumentsBeforeConnecting(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://private-secret@invalid")
	for _, args := range [][]string{{"--apply"}, {"--apply", "--actor-id", "invalid", "--reason", "test"}, {"--apply", "--actor-id", "10000000-0000-4000-8000-000000000001"}, {"--limit", "0"}, {"--limit", "1001"}, {"--older-than", "1s"}, {"--timeout", "0s"}, {"--timeout", "6m"}, {"unexpected"}} {
		if err := run(args, &bytes.Buffer{}); err == nil || strings.Contains(err.Error(), "private-secret") {
			t.Fatal("invalid or unsafe validation", args, err)
		}
	}
}
func TestPreviewRequiresOnlyReadTargetConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("MINIO_BUCKET", "")
	if err := run(nil, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatal("missing database config not identified", err)
	}
}
