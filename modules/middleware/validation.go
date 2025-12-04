// Copyright 2025 Nguyen Nhat Nguyen
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package middleware

import (
	"context"
	"io/fs"
	"net/http"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

// ValidationErrorHandler handles OpenAPI validation errors and writes an appropriate response.
type ValidationErrorHandler func(ctx context.Context, err error, w http.ResponseWriter, r *http.Request, statusCode int)

// SpecLoadErrorHandler handles errors that occur when loading the OpenAPI spec.
type SpecLoadErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

// specCache holds cached OpenAPI specs keyed by file path.
var (
	specCacheMu sync.RWMutex
	specCache   = make(map[specCacheKey]*specCacheEntry)
)

type specCacheKey struct {
	// if you care about multiple FS, you can add an ID here;
	// if not, path is probably enough.
	path string
}

type specCacheEntry struct {
	doc *openapi3.T
	err error
}

func loadSpec(fsys fs.FS, specPath string) (*openapi3.T, error) {
	key := specCacheKey{path: specPath}

	// Check cache
	specCacheMu.RLock()
	if entry, ok := specCache[key]; ok {
		specCacheMu.RUnlock()
		return entry.doc, entry.err
	}
	specCacheMu.RUnlock()

	specCacheMu.Lock()
	defer specCacheMu.Unlock()

	if entry, ok := specCache[key]; ok {
		return entry.doc, entry.err
	}

	// Read from fs.FS (embed.FS, os.DirFS, etc.)
	data, err := fs.ReadFile(fsys, specPath)
	if err != nil {
		specCache[key] = &specCacheEntry{doc: nil, err: err}
		return nil, err
	}

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	// If you need $ref with relative paths, consider LoadFromDataWithPath
	doc, err := loader.LoadFromData(data)

	specCache[key] = &specCacheEntry{doc: doc, err: err}
	return doc, err
}

// OpenAPIValidation creates a middleware that validates requests against an OpenAPI spec.
// The errorHandler is called when validation fails.
// The loadErrorHandler is called when the spec fails to load.
// TODO: use FS abstraction to not reply on specPath string which is brittle
func OpenAPIValidation(
	specFS fs.FS,
	specPath string,
	errorHandler ValidationErrorHandler,
	loadErrorHandler SpecLoadErrorHandler,
) func(http.Handler) http.Handler {
	spec, err := loadSpec(specFS, specPath)
	if err != nil {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				loadErrorHandler(w, r, err)
			})
		}
	}

	opts := &nethttpmiddleware.Options{
		Options:               openapi3filter.Options{MultiError: true},
		DoNotValidateServers:  true,
		SilenceServersWarning: true,
		ErrorHandlerWithOpts: func(ctx context.Context, err error, w http.ResponseWriter, r *http.Request, eopts nethttpmiddleware.ErrorHandlerOpts) {
			status := eopts.StatusCode
			if status == 0 {
				status = http.StatusBadRequest
			}
			// Body schema violations should be 422
			if hint := InferBodyValidationStatus(err); hint == http.StatusUnprocessableEntity {
				status = http.StatusUnprocessableEntity
			}
			errorHandler(ctx, err, w, r, status)
		},
	}

	return nethttpmiddleware.OapiRequestValidatorWithOptions(spec, opts)
}
