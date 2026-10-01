package chat

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// Store is the projects and the conversations, in the chat schema of the same
// database as the rest of goddard.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

const projectColumns = "id, slug, name, created_by"

// Projects is every project that is alive, by name.
func (s *Store) Projects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+projectColumns+" FROM chat.projects WHERE deleted_at IS NULL ORDER BY name, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projects := []Project{}
	for rows.Next() {
		var project Project
		if err := rows.Scan(&project.ID, &project.Slug, &project.Name, &project.CreatedBy); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

// Project finds one by id.
func (s *Store) Project(ctx context.Context, id string) (Project, bool, error) {
	var project Project
	err := s.db.QueryRowContext(ctx,
		"SELECT "+projectColumns+" FROM chat.projects WHERE id = $1 AND deleted_at IS NULL", id).
		Scan(&project.ID, &project.Slug, &project.Name, &project.CreatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, false, nil
	}
	if err != nil {
		return Project{}, false, err
	}
	return project, true, nil
}

// CreateProject opens one. The slug comes from the name and is what the
// directory of the workspace and the other tables use, so it is taken once and
// renaming the project later does not move it.
func (s *Store) CreateProject(ctx context.Context, name, createdBy string) (Project, error) {
	slug := Slug(name)
	if slug == "" {
		return Project{}, errors.New("ese proyecto no tiene nombre")
	}
	project := Project{Slug: slug, Name: name, CreatedBy: createdBy}
	err := s.db.QueryRowContext(ctx,
		"INSERT INTO chat.projects (slug, name, created_by) VALUES ($1, $2, $3) RETURNING id",
		slug, name, createdBy).Scan(&project.ID)
	if taken(err) {
		return Project{}, ErrTaken
	}
	if err != nil {
		return Project{}, err
	}
	return project, nil
}

// RenameProject changes what it is called. The slug stays.
func (s *Store) RenameProject(ctx context.Context, id, name string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE chat.projects SET name = $2, updated_at = goddard.now() WHERE id = $1 AND deleted_at IS NULL",
		id, name)
	return err
}

// DeleteProject takes it out of the list. It is marked, not dropped: the
// conversations and the memory of what happened keep pointing at it.
func (s *Store) DeleteProject(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE chat.projects SET deleted_at = goddard.now(), updated_at = goddard.now() "+
			"WHERE id = $1 AND deleted_at IS NULL", id)
	return err
}

const conversationColumns = "id, project_id, title, source, created_by, COALESCE(claimed_until, 0), updated_at"

func scanConversation(row interface{ Scan(...any) error }) (Conversation, error) {
	var conversation Conversation
	err := row.Scan(&conversation.ID, &conversation.ProjectID, &conversation.Title, &conversation.Source,
		&conversation.CreatedBy, &conversation.ClaimedUntil, &conversation.UpdatedAt)
	return conversation, err
}

// Conversations is what is alive inside a project, most recently touched
// first.
func (s *Store) Conversations(ctx context.Context, projectID string) ([]Conversation, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+conversationColumns+" FROM chat.conversations "+
			"WHERE project_id = $1 AND deleted_at IS NULL ORDER BY updated_at DESC, id DESC", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	conversations := []Conversation{}
	for rows.Next() {
		conversation, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}
	return conversations, rows.Err()
}

// Conversation finds one by id.
func (s *Store) Conversation(ctx context.Context, id string) (Conversation, bool, error) {
	conversation, err := scanConversation(s.db.QueryRowContext(ctx,
		"SELECT "+conversationColumns+" FROM chat.conversations WHERE id = $1 AND deleted_at IS NULL", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Conversation{}, false, nil
	}
	if err != nil {
		return Conversation{}, false, err
	}
	return conversation, true, nil
}

// CreateConversation opens a thread. Without a title it is the one goddard has
// always used for one that just started, and the first message renames it.
func (s *Store) CreateConversation(ctx context.Context, projectID, title, source, createdBy string) (Conversation, error) {
	if source == "" {
		source = SourceWeb
	}
	if title == "" {
		title = NewTitle
	}
	conversation := Conversation{ProjectID: projectID, Title: title, Source: source, CreatedBy: createdBy}
	err := s.db.QueryRowContext(ctx,
		"INSERT INTO chat.conversations (project_id, title, source, created_by) VALUES ($1, $2, $3, $4) RETURNING id",
		projectID, title, source, createdBy).Scan(&conversation.ID)
	if err != nil {
		return Conversation{}, err
	}
	return conversation, nil
}

// RenameConversation changes what the thread is called.
func (s *Store) RenameConversation(ctx context.Context, id, title string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE chat.conversations SET title = $2, updated_at = goddard.now() "+
			"WHERE id = $1 AND deleted_at IS NULL", id, title)
	return err
}

// DeleteConversation takes the thread out of the list.
func (s *Store) DeleteConversation(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE chat.conversations SET deleted_at = goddard.now(), updated_at = goddard.now() "+
			"WHERE id = $1 AND deleted_at IS NULL", id)
	return err
}

func taken(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
