package domain

import (
	"strconv"
	"time"

	"app/modules/db"

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
		Age       int
		CreatedAt time.Time

		Version int64
	}
)

func (p *Profile) V() string {
	return strconv.Itoa(int(p.Version))
}

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
