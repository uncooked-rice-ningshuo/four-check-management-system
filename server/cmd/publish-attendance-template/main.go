// Command publish-attendance-template previews or explicitly publishes the
// supported attendance v1 workbook. It does not migrate or bootstrap accounts.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/importjob"
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

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("publish-attendance-template", flag.ContinueOnError)
	flags.SetOutput(output)
	typeCode := flags.String("type-code", "ATTENDANCE_RATE", "ATTENDANCE_RATE, UNATTENDED_CLASSROOM, MORNING_ATTENDANCE, LATE_RETURN, DORMITORY_HYGIENE, CLASSROOM_HYGIENE")
	apply := flags.Bool("apply", false, "publish to configured PostgreSQL/MinIO (default: preview only)")
	actor := flags.String("actor-id", "", "active super administrator UUID for audit attribution")
	reason := flags.String("reason", "", "publication reason (required with --apply)")
	path := flags.String("output", "", "write a new local XLSX file; existing paths are refused")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("不接受额外位置参数")
	}
	if *apply {
		if _, err := uuid.Parse(*actor); err != nil || strings.TrimSpace(*reason) == "" {
			return errors.New("发布必须提供有效 --actor-id 和 --reason")
		}
	}
	definition, err := importjob.InspectionTemplate(*typeCode)
	if err != nil {
		return errors.New("不支持的检查类型")
	}
	content, err := importjob.BuildInspectionTemplate(*typeCode)
	if err != nil {
		return errors.New("模板生成失败")
	}
	parser, err := importjob.NewInspectionXLSXParser(*typeCode, 16<<20)
	if err != nil {
		return errors.New("不支持的检查类型")
	}
	if _, err := parser.Parse(definition.Filename, content); err != nil {
		return errors.New("模板契约校验失败")
	}
	if *path != "" {
		file, err := os.OpenFile(*path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return errors.New("无法创建输出文件；请确认目录存在且文件尚不存在")
		}
		_, writeErr := file.Write(content)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			return errors.New("模板文件写入失败")
		}
	}
	if !*apply {
		return json.NewEncoder(output).Encode(map[string]any{"mode": "preview", "code": definition.Code, "version": 1, "sizeBytes": len(content), "sha256": fmt.Sprintf("%x", sha256.Sum256(content))})
	}
	for _, key := range []string{"DATABASE_URL", "MINIO_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_BUCKET", "MINIO_USE_TLS"} {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			return fmt.Errorf("缺少配置 %s", key)
		}
	}
	tls, err := strconv.ParseBool(os.Getenv("MINIO_USE_TLS"))
	if err != nil {
		return errors.New("MINIO_USE_TLS 无效")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := store.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return errors.New("数据库连接失败，请核对目标配置")
	}
	defer db.Close()
	if err := db.Ready(ctx); err != nil {
		return errors.New("数据库结构未就绪，请先核对迁移")
	}
	objects, err := storage.NewMinIO(storage.MinIOConfig{Endpoint: os.Getenv("MINIO_ENDPOINT"), AccessKey: os.Getenv("MINIO_ACCESS_KEY"), SecretKey: os.Getenv("MINIO_SECRET_KEY"), Bucket: os.Getenv("MINIO_BUCKET"), Region: os.Getenv("MINIO_REGION"), UseTLS: tls, ConnectTimeout: 3 * time.Second, OperationTimeout: 10 * time.Second})
	if err != nil {
		return errors.New("对象存储配置无效")
	}
	if err := objects.Ready(ctx); err != nil {
		return errors.New("对象存储未就绪，请核对应用账号和 Bucket")
	}
	files := storage.NewService(db, objects, 16<<20)
	result, err := importjob.NewInspectionTemplatePublisher(db, files, *typeCode).Publish(ctx, audit.Metadata{ActorID: *actor, Reason: *reason, RequestID: uuid.NewString()})
	if errors.Is(err, auth.ErrForbidden) {
		return errors.New("审计账号必须是启用中的超级管理员")
	}
	if errors.Is(err, importjob.ErrTemplatePublicationConflict) {
		return errors.New("已存在不同或未发布的模板版本；拒绝覆盖，请核对后制定修订方案")
	}
	if err != nil {
		return errors.New("模板发布或完整性校验失败；未覆盖既有版本，请核对文件状态及审计后重试")
	}
	return json.NewEncoder(output).Encode(map[string]any{"mode": "published", "template": result, "sha256": fmt.Sprintf("%x", sha256.Sum256(content))})
}
