package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jmechavez/vapeshop/internal/database"
)

var ErrUserNotFound = errors.New("user not found")

// FindByUsername looks up a user by username.
// Returns ErrUserNotFound if no row matches.
func FindByUsername(ctx context.Context, username string) (*User, error) {
	row := database.Pool.QueryRow(ctx,
		`SELECT id, username, password_hash, full_name, role, active, created_at, updated_at
		 FROM users WHERE username = $1`, username)

	var u User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.FullName,
		&u.Role, &u.Active, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}
