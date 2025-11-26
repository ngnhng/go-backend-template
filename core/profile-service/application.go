package profile_service

import "app/db"

func newApp(pool db.ConnectionPool, persistence ProfilePersistence, signer CursorSigner) *Application {
	return &Application{pool: pool, persistence: persistence, signer: signer}
}
