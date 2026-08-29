package storage

import (
	"reflect"
	"testing"

	"github.com/minio/minio-go/v7"
)

func TestMinIOSDKExposesRequiredObjectOperations(t *testing.T) {
	clientType := reflect.TypeOf((*minio.Client)(nil))
	for _, method := range []string{"PutObject", "GetObject", "StatObject", "RemoveObject"} {
		if _, exists := clientType.MethodByName(method); !exists {
			t.Errorf("MinIO SDK does not expose %s", method)
		}
	}
}
