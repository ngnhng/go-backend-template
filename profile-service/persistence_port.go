package profile_service

import (
	"app/db"
	"context"
	"time"

	"github.com/gofrs/uuid/v5"
)

// ProfilePersistence is the outbound port that storage adapters (e.g., Postgres) implement.
// Note: We pass db.Querier for read/write routing at the application layer.
type ProfilePersistence interface {
	CreateProfile(context.Context, db.Querier, string, string) (*Profile, error)
	GetProfilesByOffset(context.Context, db.Querier, int, int) ([]Profile, int, error)
	GetProfilesByCursor(context.Context, db.Querier, time.Time, uuid.UUID, CursorDirection, int) ([]Profile, error)
	GetProfilesFirstPage(context.Context, db.Querier, int) ([]Profile, error)
	GetProfileByID(context.Context, db.Querier, uuid.UUID) (*Profile, error)
	UpdateProfile(context.Context, db.Querier, uuid.UUID, string, *string) (*Profile, error)
	ModifyProfile(context.Context, db.Querier, uuid.UUID, bool, bool, string, bool, bool, int32, bool, string) (*Profile, error)
	DeleteProfile(context.Context, db.Querier, uuid.UUID) error
}
