package chat

import (
	"context"
	"database/sql"
	"errors"
)

// Upload is an attached file of a conversation. It lives in the database and
// not on a disk because any instance serves any conversation: a file in one
// instance's filesystem is a file the others cannot show.
type Upload struct {
	ID    string
	Name  string
	Mime  string
	Bytes []byte
}

// PutUpload stores a file and returns it with the id it got.
func (s *Store) PutUpload(ctx context.Context, conversationID, name, mime string, bytes []byte) (Upload, error) {
	upload := Upload{Name: name, Mime: mime, Bytes: bytes}
	err := s.db.QueryRowContext(ctx,
		"INSERT INTO chat.uploads (conversation_id, name, mime, bytes) VALUES ($1, $2, $3, $4) RETURNING id",
		conversationID, name, mime, bytes).Scan(&upload.ID)
	if err != nil {
		return Upload{}, err
	}
	return upload, nil
}

// Upload reads one with its bytes, which is what sending a message needs.
func (s *Store) Upload(ctx context.Context, id string) (Upload, bool, error) {
	var upload Upload
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, mime, bytes FROM chat.uploads WHERE id = $1 AND deleted_at IS NULL", id).
		Scan(&upload.ID, &upload.Name, &upload.Mime, &upload.Bytes)
	if errors.Is(err, sql.ErrNoRows) {
		return Upload{}, false, nil
	}
	if err != nil {
		return Upload{}, false, err
	}
	return upload, true, nil
}

// Uploads names what a conversation has attached, without the bytes: a list
// that carries every image is not a list.
func (s *Store) Uploads(ctx context.Context, conversationID string) ([]Upload, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, mime FROM chat.uploads WHERE conversation_id = $1 AND deleted_at IS NULL ORDER BY created_at, id",
		conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	uploads := []Upload{}
	for rows.Next() {
		var upload Upload
		if err := rows.Scan(&upload.ID, &upload.Name, &upload.Mime); err != nil {
			return nil, err
		}
		uploads = append(uploads, upload)
	}
	return uploads, rows.Err()
}
