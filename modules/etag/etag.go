package etag

import (
	"fmt"
	"strings"
)

type ETaggable interface {
	V() string
}

// For HTTP headers, remember that the actual header value is usually quoted:
//
// fmt.Sprintf("%q", ETag(obj))
func ETag(obj ETaggable) string {
	return "v:" + obj.V()
}

func ParseETag(etag string) (string, error) {
	const prefix = "v:"
	if !strings.HasPrefix(etag, prefix) {
		return "", fmt.Errorf("invalid etag format")
	}
	return strings.TrimPrefix(etag, prefix), nil
}
