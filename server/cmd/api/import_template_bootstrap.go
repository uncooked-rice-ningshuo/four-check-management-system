package main

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/storage"
	"four-check-management-system/server/internal/store"
	"github.com/google/uuid"
)

const importTemplateBootstrapReason = "服务启动自动初始化导入模板"

// Import templates are runtime artifacts: the workbook lives in the object
// store and its publication row in PostgreSQL. A freshly migrated database
// therefore has no downloadable template, which used to make some inspection
// entries fail with 404 until an operator ran the publisher CLI.
//
// Publishing from the running service keeps every entry equally usable. The
// work stays asynchronous so a slow object store never delays readiness, and it
// is idempotent because the publisher verifies an already published v1 instead
// of overwriting it.
func startImportTemplateBootstrap(db *store.Postgres, files *storage.Service) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		for attempt := 1; attempt <= 3; attempt++ {
			err := ensurePublishedImportTemplates(ctx, db, files)
			if err == nil {
				log.Printf("import template bootstrap: all supported templates are published")
				return
			}
			log.Printf("import template bootstrap attempt %d failed: %v", attempt, err)
			select {
			case <-ctx.Done():
				log.Printf("import template bootstrap cancelled: %v", ctx.Err())
				return
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		log.Printf("import template bootstrap gave up; affected inspection entries stay unavailable until a publisher runs")
	}()
}

func ensurePublishedImportTemplates(ctx context.Context, db *store.Postgres, files *storage.Service) error {
	actorID, err := db.FindTemplatePublisherActor(ctx)
	if err != nil {
		return err
	}
	failures := make([]string, 0)
	for _, typeCode := range importjob.PublishedTemplateTypeCodes() {
		publisher := importjob.NewInspectionTemplatePublisher(db, files, typeCode)
		metadata := audit.Metadata{ActorID: actorID, RequestID: uuid.NewString(), Reason: importTemplateBootstrapReason + " " + typeCode + " v1"}
		if _, err := publisher.Publish(ctx, metadata); err != nil {
			failures = append(failures, typeCode+": "+err.Error())
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}
