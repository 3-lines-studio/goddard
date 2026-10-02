package auth

import (
	"context"
	"database/sql"
	"errors"
)

// Store is the users, the links and the sessions, in the auth schema of the
// same database as the rest of goddard.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

const userColumns = "u.id, u.email, u.name"

// User finds somebody by email. The email is unique and lowercase, and that is
// the caller's business: this looks for what it is given.
func (s *Store) User(ctx context.Context, email string) (User, bool, error) {
	var user User
	err := s.db.QueryRowContext(ctx,
		"SELECT "+userColumns+" FROM auth.users u WHERE u.email = $1 AND u.deleted_at IS NULL", email).
		Scan(&user.ID, &user.Email, &user.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return user, true, nil
}

// ByID finds somebody by the id the database minted, which is what the rest of
// goddard keeps. The email is how they come in and can change; the id cannot.
func (s *Store) ByID(ctx context.Context, id string) (User, bool, error) {
	var user User
	err := s.db.QueryRowContext(ctx,
		"SELECT "+userColumns+" FROM auth.users u WHERE u.id = $1 AND u.deleted_at IS NULL", id).
		Scan(&user.ID, &user.Email, &user.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return user, true, nil
}

// SetName changes what goddard calls somebody. The name that comes with the
// email is a guess — the local part of the mail — and this is where it stops
// being one: it is what the prompt says and what the app shows.
func (s *Store) SetName(ctx context.Context, id, name string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE auth.users SET name = $1, updated_at = goddard.now() WHERE id = $2 AND deleted_at IS NULL",
		name, id)
	return err
}

// CreateLogin mints a link for an email, creating the user the first time, and
// returns what goes in it. The link is not stored: only its hash.
func (s *Store) CreateLogin(ctx context.Context, email string, ttl int64) (string, error) {
	plain, err := randomHex(24)
	if err != nil {
		return "", err
	}
	err = s.write(ctx, func(tx *sql.Tx) error {
		var id string
		err := tx.QueryRowContext(ctx,
			"SELECT id FROM auth.users WHERE email = $1 AND deleted_at IS NULL", email).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			err = tx.QueryRowContext(ctx,
				"INSERT INTO auth.users (email, name) VALUES ($1, $2) RETURNING id", email, nameOf(email)).Scan(&id)
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			"INSERT INTO auth.logins (user_id, hash, expires_at) VALUES ($1, $2, goddard.now() + $3)",
			id, hash(plain), ttl)
		return err
	})
	if err != nil {
		return "", err
	}
	return plain, nil
}

// ConsumeLogin burns a link and says who it was for. A link that is not there,
// one that expired and one that was already used are all the same answer.
func (s *Store) ConsumeLogin(ctx context.Context, plain string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx,
		"SELECT "+userColumns+" FROM auth.logins l JOIN auth.users u ON u.id = l.user_id "+
			"WHERE l.hash = $1 AND l.expires_at > goddard.now() AND l.deleted_at IS NULL", hash(plain)).
		Scan(&user.ID, &user.Email, &user.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, errors.New("ese link ya no sirve")
	}
	if err != nil {
		return User{}, err
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM auth.logins WHERE hash = $1", hash(plain)); err != nil {
		return User{}, err
	}
	return user, nil
}

// AskedRecently says whether that email asked for a link within the window, so
// the caller can leave the request alone instead of mailing again.
func (s *Store) AskedRecently(ctx context.Context, email string, within int64) (bool, error) {
	var last *int64
	err := s.db.QueryRowContext(ctx,
		"SELECT MAX(l.created_at) FROM auth.logins l JOIN auth.users u ON u.id = l.user_id "+
			"WHERE u.email = $1 AND l.deleted_at IS NULL", email).Scan(&last)
	if err != nil {
		return false, err
	}
	var now int64
	if err := s.db.QueryRowContext(ctx, "SELECT goddard.now()").Scan(&now); err != nil {
		return false, err
	}
	return last != nil && now-*last < within, nil
}

// CreateSession opens a session for a user and returns what goes in the
// cookie. The session is not stored either: only its hash.
func (s *Store) CreateSession(ctx context.Context, userID string, ttl int64) (string, error) {
	plain, err := randomHex(24)
	if err != nil {
		return "", err
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO auth.sessions (user_id, hash, expires_at) VALUES ($1, $2, goddard.now() + $3)",
		userID, hash(plain), ttl); err != nil {
		return "", err
	}
	return plain, nil
}

// Session says who is behind a cookie, or nothing at all when it is not there
// or has expired.
func (s *Store) Session(ctx context.Context, plain string) (User, bool, error) {
	var user User
	err := s.db.QueryRowContext(ctx,
		"SELECT "+userColumns+" FROM auth.sessions s JOIN auth.users u ON u.id = s.user_id "+
			"WHERE s.hash = $1 AND s.expires_at > goddard.now() AND s.deleted_at IS NULL", hash(plain)).
		Scan(&user.ID, &user.Email, &user.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return user, true, nil
}

// DropSession signs somebody out.
func (s *Store) DropSession(ctx context.Context, plain string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM auth.sessions WHERE hash = $1", hash(plain))
	return err
}

// Sweep takes out what expired. Links and sessions are not history: they are
// dropped, not marked.
func (s *Store) Sweep(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM auth.logins WHERE expires_at <= goddard.now()"); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM auth.sessions WHERE expires_at <= goddard.now()")
	return err
}

func (s *Store) write(ctx context.Context, work func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := work(tx); err != nil {
		return err
	}
	return tx.Commit()
}
