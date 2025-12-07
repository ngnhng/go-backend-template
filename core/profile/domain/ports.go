// Copyright 2025 Nhat-Nguyen Nguyen
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

package domain

import (
	"context"
	"time"

	"github.com/gofrs/uuid/v5"
)

// ProfileReadStore defines the port for read operations on profiles.
//
// Read/Write Separation Pattern:
// This interface is separated from ProfileWriteStore to enable:
//   - Read-replica routing: implementations can route reads to replica databases
//   - Prepared statement optimization: read queries can be prepared once and reused
//   - CQRS patterns: different read models can be optimized for specific query patterns
//
// Implementation Notes:
//   - Implementations should be bound to a read-replica database connection
//   - Queries should use prepared statements where possible for performance
//   - All methods are read-only and should never modify data
//   - No transaction support needed (reads don't require atomicity across operations)
//
// TODO: ProfileReadTx?
type ProfileReadStore interface {
	// GetProfilesByCursor implements cursor-based pagination using a keyset approach.
	// The cursor contains a pivot point (created_at, id) and direction (ASC/DESC).
	// This method is more efficient than offset-based pagination for large datasets
	// because it uses indexed columns and doesn't require scanning skipped rows.
	//
	// Parameters:
	//   - pivotCreatedAt: The created_at timestamp of the last item from previous page
	//   - pivotID: The ID of the last item from previous page (for tie-breaking)
	//   - dir: Direction to paginate (ASC for next page, DESC for previous page)
	//   - limit: Maximum number of items to return
	//
	// Returns profiles ordered by (created_at DESC, id DESC) regardless of direction.
	// The direction only affects the comparator used in the WHERE clause.
	GetProfilesByCursor(ctx context.Context, pivotCreatedAt time.Time, pivotID uuid.UUID, dir CursorDirection, limit int) ([]Profile, error)

	// GetProfilesFirstPage returns the first page for cursor-based pagination.
	// This is used when the client doesn't provide a cursor (initial page load).
	// Results are ordered by (created_at DESC, id DESC) to match cursor pagination order.
	GetProfilesFirstPage(ctx context.Context, limit int) ([]Profile, error)

	// GetProfilesByOffset implements traditional offset-based pagination.
	// Returns both the page of profiles and the total count.
	//
	// Note: Offset pagination has performance issues with large offsets because
	// the database must scan and discard all skipped rows. Use cursor-based
	// pagination (GetProfilesByCursor) for better performance on large datasets.
	//
	// Returns: (profiles, totalCount, error)
	GetProfilesByOffset(ctx context.Context, limit, offset int) ([]Profile, int, error)

	// GetProfileByID retrieves a single profile by its unique identifier.
	// Returns ErrProfileNotFound if the profile doesn't exist or is soft-deleted.
	GetProfileByID(ctx context.Context, id uuid.UUID) (*Profile, error)
}

// ProfileWriteStore defines the port for write operations on profiles.
//
// Write Operations Pattern:
// This interface is separated from ProfileReadStore to enable:
//   - Primary database routing: all writes go to the primary database
//   - Transaction support: writes can be grouped into atomic transactions via WithTx
//   - Optimistic concurrency: version fields prevent lost updates
//   - Prepared statement optimization: write queries can be prepared once and reused
//
// Transaction Handling:
// All methods on this interface execute WITHOUT an implicit transaction.
// To group multiple operations atomically, use the WithTx method which provides
// a ProfileWriteTx scoped to the transaction lifetime.
//
// Optimistic Concurrency:
// UpdateProfile, ModifyProfile, and DeleteProfile require a version number.
// If the version doesn't match the current database version, the operation fails
// with ErrPrecondition, indicating another client has modified the entity.
//
// Implementation Notes:
//   - Implementations should be bound to the primary database connection
//   - Write operations should use prepared statements where possible
//   - All mutations should support optimistic locking via version fields
//   - Soft deletes are preferred over hard deletes for audit trails
type ProfileWriteStore interface {
	// CreateProfile inserts a new profile with the given username and email.
	// Returns the created profile with auto-generated ID, version, and timestamps.
	//
	// Validation:
	//   - Username must be non-empty
	//   - Email must be unique (enforced by database constraint)
	//
	// Returns ErrDuplicateProfile if a profile with the same email already exists.
	CreateProfile(ctx context.Context, username, email string) (*Profile, error)

	// UpdateProfile performs a full update of the profile's username and email.
	// Uses optimistic concurrency control via the version field.
	//
	// The version number is automatically incremented on successful update.
	// If the provided version doesn't match the current database version,
	// returns ErrPrecondition (indicating a concurrent modification).
	//
	// Returns:
	//   - The updated profile with new version number
	//   - ErrPrecondition if version mismatch (concurrent update detected)
	//   - ErrDuplicateProfile if email conflicts with another profile
	//   - ErrProfileNotFound if profile doesn't exist or is deleted
	UpdateProfile(ctx context.Context, params *UpdateProfileParams) (*Profile, error)

	// DeleteProfile performs a soft delete by setting deleted_at to current timestamp.
	// Uses optimistic concurrency control via the version field.
	//
	// Soft Delete Benefits:
	//   - Audit trail preservation
	//   - Ability to restore deleted data
	//   - Foreign key constraints remain intact
	//
	// The version number is checked and incremented atomically.
	// Returns ErrPrecondition if version mismatch or ErrProfileNotFound if not found.
	DeleteProfile(ctx context.Context, id uuid.UUID, version int64) error

	// ModifyProfile performs a partial update (PATCH semantics).
	// Only the fields marked as "set" will be updated; others remain unchanged.
	//
	// Field Update Semantics:
	//   - If nameSet=false: name field is not touched
	//   - If nameSet=true, nameNull=true: name is set to NULL
	//   - If nameSet=true, nameNull=false: name is set to nameVal
	//   - Same pattern applies for age field
	//   - Email field doesn't support NULL, so only emailSet matters
	//
	// Use Cases:
	//   - Client wants to update only age without changing name/email
	//   - Client wants to clear an optional field (set to NULL)
	//   - Implementing JSON Merge Patch (RFC 7386) semantics
	//
	// This method is left unprepared because the SQL SET clause is dynamic
	// based on which fields are being updated.
	//
	// Returns ErrPrecondition if version mismatch, ErrProfileNotFound if not found.
	ModifyProfile(
		ctx context.Context,
		id uuid.UUID,
		version int64,
		nameSet, nameNull bool, nameVal string,
		ageSet, ageNull bool, ageVal int32,
		emailSet bool, emailVal string,
	) (*Profile, error)

	// WithTx executes the given function within a database transaction.
	//
	// Transaction Pattern:
	//   - Automatically handles BEGIN/COMMIT/ROLLBACK
	//   - If fn returns an error, transaction is rolled back
	//   - If fn returns nil, transaction is committed
	//   - Panics are caught and trigger rollback
	//
	// The provided ProfileWriteTx uses the same prepared statements as the
	// parent ProfileWriteStore, but bound to the transaction connection.
	// This allows prepared statement reuse even within transactions.
	//
	// Example Usage:
	//   err := writer.WithTx(ctx, func(ctx context.Context, tx ProfileWriteTx) error {
	//       profile, err := tx.CreateProfile(ctx, "alice", "alice@example.com")
	//       if err != nil {
	//           return err // triggers rollback
	//       }
	//       // Do more work...
	//       return nil // triggers commit
	//   })
	//
	// Important: Do NOT nest WithTx calls - the ProfileWriteTx interface
	// intentionally does not expose WithTx to prevent nested transactions.
	WithTx(ctx context.Context, fn func(ctx context.Context, tx ProfileWriteTx) error) error
	// WithTimeoutTx is the same as WithTx but applies a context timeout before starting the transaction.
	WithTimeoutTx(ctx context.Context, timeout time.Duration, fn func(ctx context.Context, tx ProfileWriteTx) error) error
}

// ProfileWriteTx is a transaction-scoped version of ProfileWriteStore.
//
// Lifecycle:
//   - Created by ProfileWriteStore.WithTx()
//   - Bound to a specific database transaction
//   - Automatically cleaned up when the transaction completes
//
// Key Differences from ProfileWriteStore:
//   - No WithTx method (transactions cannot be nested)
//   - All operations execute within the same transaction
//   - Prepared statements are reused from parent but bound to transaction connection
//
// Prepared Statement Reuse:
// Implementations can reuse prepared statements from the parent ProfileWriteStore
// by rebinding them to the transaction connection using bob.InTx(). This provides
// the performance benefits of prepared statements even within transactions.
//
// Thread Safety:
// A ProfileWriteTx instance is NOT thread-safe and should only be used by the
// function that received it from WithTx. Do not pass it to goroutines.
type ProfileWriteTx interface {
	// CreateProfile inserts a new profile within the transaction.
	// See ProfileWriteStore.CreateProfile for detailed documentation.
	CreateProfile(ctx context.Context, username, email string) (*Profile, error)

	// UpdateProfile updates a profile within the transaction.
	// See ProfileWriteStore.UpdateProfile for detailed documentation.
	UpdateProfile(ctx context.Context, params *UpdateProfileParams) (*Profile, error)

	// DeleteProfile soft-deletes a profile within the transaction.
	// See ProfileWriteStore.DeleteProfile for detailed documentation.
	DeleteProfile(ctx context.Context, id uuid.UUID, version int64) error

	// ModifyProfile performs a partial update within the transaction.
	// See ProfileWriteStore.ModifyProfile for detailed documentation.
	ModifyProfile(
		ctx context.Context,
		id uuid.UUID,
		version int64,
		nameSet, nameNull bool, nameVal string,
		ageSet, ageNull bool, ageVal int32,
		emailSet bool, emailVal string,
	) (*Profile, error)
}
