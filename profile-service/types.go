package profile_service

import (
	"app/db"
	"database/sql"
	"time"

	"github.com/gofrs/uuid/v5"
)

type (
	Application struct {
		pool        db.ConnectionPool
		persistence ProfilePersistence
		signer      CursorSigner
	}

	// Profile is the domain model used by the application layer.
	Profile struct {
		ID        uuid.UUID
		Name      string
		Email     string
		Age       sql.NullInt32
		CreatedAt time.Time
	}

	// ProfileRow is the persistence entity shape used by storage adapters.
	ProfileRow struct {
		ID        uuid.UUID     `db:"id"`
		Name      string        `db:"username"`
		Email     string        `db:"email"`
		Age       sql.NullInt32 `db:"age"`
		CreatedAt time.Time     `db:"created_at"`
	}
)

const (
	ASC  CursorDirection = "asc"
	DESC CursorDirection = "desc"
)

type (
	CursorDirection string

	CursorPaginationToken struct {
		TTL       time.Time       `json:"ttl"`
		Direction CursorDirection `json:"direction"`

		Pivot struct {
			CreatedAt time.Time `json:"created_at"`
			ID        uuid.UUID `json:"id"`
		} `json:"pivot"`

		Signature string `json:"-"`
	}
)
