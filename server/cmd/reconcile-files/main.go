// Command reconcile-files previews or explicitly executes one bounded file recovery batch.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/storage"
	"four-check-management-system/server/internal/store"
	"github.com/google/uuid"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type candidateSummary struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	SizeBytes int64     `json:"sizeBytes"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("reconcile-files", flag.ContinueOnError)
	flags.SetOutput(output)
	apply := flags.Bool("apply", false, "execute recovery (default: read-only preview)")
	actorID := flags.String("actor-id", "", "active super administrator UUID")
	reason := flags.String("reason", "", "audit reason required with --apply")
	limit := flags.Int("limit", 100, "maximum candidates in one batch, 1-1000")
	olderThan := flags.Duration("older-than", time.Hour, "minimum inactivity, at least 5m")
	timeout := flags.Duration("timeout", time.Minute, "overall deadline, 1s-5m")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *limit < 1 || *limit > 1000 || *olderThan < 5*time.Minute || *timeout < time.Second || *timeout > 5*time.Minute {
		return errors.New("参数无效：limit须1–1000，older-than至少5分钟，timeout须1秒–5分钟，不接受位置参数")
	}
	if *apply {
		if _, err := uuid.Parse(*actorID); err != nil || strings.TrimSpace(*reason) == "" || utf8.RuneCountInString(*reason) > 255 {
			return errors.New("执行必须提供有效actor-id和1–255字符的reason")
		}
	}
	for _, key := range []string{"DATABASE_URL", "MINIO_BUCKET"} {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			return fmt.Errorf("缺少配置 %s", key)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	database, err := store.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return errors.New("数据库连接失败，请核对目标配置")
	}
	defer database.Close()
	if err := database.Ready(ctx); err != nil {
		return errors.New("数据库结构未就绪，请先核对迁移")
	}
	cutoff := time.Now().UTC().Add(-*olderThan)
	bucket := os.Getenv("MINIO_BUCKET")
	if !*apply {
		files, err := database.ListFileReconciliationCandidates(ctx, bucket, cutoff, *limit)
		if err != nil {
			return errors.New("无法读取恢复候选，请核对数据库状态")
		}
		candidates := make([]candidateSummary, 0, len(files))
		for _, file := range files {
			candidates = append(candidates, candidateSummary{ID: file.ID, Status: file.Status, SizeBytes: file.SizeBytes, UpdatedAt: file.UpdatedAt})
		}
		return json.NewEncoder(output).Encode(map[string]any{"mode": "preview", "before": cutoff, "limit": *limit, "count": len(candidates), "candidates": candidates})
	}
	actor, err := database.FindByID(ctx, *actorID)
	if errors.Is(err, auth.ErrAccountNotFound) {
		return errors.New("操作账号必须是启用中且已完成改密的超级管理员")
	}
	if err != nil {
		return errors.New("无法核验操作账号，请稍后重试")
	}
	if actor.Status != auth.StatusActive || !actor.IsSuperAdmin || actor.MustChangePassword {
		return errors.New("操作账号必须是启用中且已完成改密的超级管理员")
	}
	for _, key := range []string{"MINIO_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_USE_TLS", "MINIO_MAX_FILE_SIZE_BYTES"} {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			return fmt.Errorf("缺少配置 %s", key)
		}
	}
	useTLS, err := strconv.ParseBool(os.Getenv("MINIO_USE_TLS"))
	if err != nil {
		return errors.New("MINIO_USE_TLS无效")
	}
	maxSize, err := strconv.ParseInt(os.Getenv("MINIO_MAX_FILE_SIZE_BYTES"), 10, 64)
	if err != nil || maxSize <= 0 {
		return errors.New("MINIO_MAX_FILE_SIZE_BYTES必须是正整数")
	}
	objects, err := storage.NewMinIO(storage.MinIOConfig{Endpoint: os.Getenv("MINIO_ENDPOINT"), AccessKey: os.Getenv("MINIO_ACCESS_KEY"), SecretKey: os.Getenv("MINIO_SECRET_KEY"), Bucket: bucket, Region: os.Getenv("MINIO_REGION"), UseTLS: useTLS, ConnectTimeout: 3 * time.Second, OperationTimeout: 10 * time.Second})
	if err != nil {
		return errors.New("对象存储配置无效")
	}
	if err := objects.Ready(ctx); err != nil {
		return errors.New("对象存储未就绪，请核对应用账号和Bucket")
	}
	result, err := storage.NewService(database, objects, maxSize).Reconcile(ctx, cutoff, *limit, audit.Metadata{ActorID: actor.ID, Reason: strings.TrimSpace(*reason), RequestID: uuid.NewString()})
	if err != nil {
		return errors.New("恢复批次查询失败，请核对数据库状态并重试")
	}
	if err := json.NewEncoder(output).Encode(map[string]any{"mode": "applied", "before": cutoff, "limit": *limit, "recovered": result.Recovered, "cleaned": result.Cleaned, "failed": result.Failed}); err != nil {
		return err
	}
	if result.Failed > 0 {
		return errors.New("部分文件未恢复，请核对审计和文件状态后重试")
	}
	return nil
}
