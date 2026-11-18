package store

import (
	"testing"
	"time"
)

type disciplineVersionRow struct{ created, published time.Time }

func (r disciplineVersionRow) Scan(dest ...any) error {
	*(dest[0].(*string)) = "version-id"
	*(dest[1].(*int)) = 2
	*(dest[2].(*string)) = "PUBLISHED"
	*(dest[3].(*int)) = 3
	*(dest[4].(*string)) = "creator-id"
	*(dest[5].(*time.Time)) = r.created
	*(dest[6].(*string)) = "规则管理员"
	publisher := "publisher-id"
	*(dest[7].(**string)) = &publisher
	*(dest[8].(**time.Time)) = &r.published
	publisherName := "发布管理员"
	*(dest[9].(**string)) = &publisherName
	return nil
}

func TestScanDisciplineVersionReturnsAccountDisplayNames(t *testing.T) {
	created := time.Date(2026, 11, 16, 1, 0, 0, 0, time.UTC)
	published := created.Add(time.Minute)
	version, err := scanDisciplineVersion(disciplineVersionRow{created: created, published: published})
	if err != nil {
		t.Fatal(err)
	}
	if version.CreatedByName != "规则管理员" || version.PublishedByName != "发布管理员" {
		t.Fatalf("expected display names in version history, got %+v", version)
	}
}
