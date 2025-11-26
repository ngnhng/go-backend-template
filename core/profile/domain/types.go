package domain

import (
	"database/sql"
	"time"

	"app/db"

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
