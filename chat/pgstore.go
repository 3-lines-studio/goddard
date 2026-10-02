package chat

import (
	"context"
	"database/sql"
	"errors"

	"github.com/3-lines-studio/goddard/naming"
)

// Store is the projects and the conversations, in the chat schema of the same
// database as the rest of goddard.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

const projectColumns = "id, slug, name, owner_kind, owner_id, created_by"

// Projects is every project alive that is this viewer's: theirs, and the ones
// of the organizations they are in, by name. A project of somebody else is
// invisible, not forbidden.
func (s *Store) Projects(ctx context.Context, viewer Owner, orgs []string) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+projectColumns+" FROM chat.projects WHERE deleted_at IS NULL AND ("+
			"(owner_kind = 'user' AND owner_id = $1) OR (owner_kind = 'org' AND owner_id = ANY($2))"+
			") ORDER BY name, id", viewer.ID, orgs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projects := []Project{}
	for rows.Next() {
		var project Project
		if err := scanProject(rows, &project); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

// Project finds one by id. It answers it whatever the owner: the caller decides
// what they may do with what they found.
func (s *Store) Project(ctx context.Context, id string) (Project, bool, error) {
	var project Project
	err := s.db.QueryRowContext(ctx,
		"SELECT "+projectColumns+" FROM chat.projects WHERE id = $1 AND deleted_at IS NULL", id).
		Scan(&project.ID, &project.Slug, &project.Name, &project.Owner.Kind, &project.Owner.ID, &project.CreatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, false, nil
	}
	if err != nil {
		return Project{}, false, err
	}
	return project, true, nil
}

func scanProject(row interface{ Scan(...any) error }, project *Project) error {
	return row.Scan(&project.ID, &project.Slug, &project.Name, &project.Owner.Kind, &project.Owner.ID, &project.CreatedBy)
}

// CreateProject opens one. The slug comes from the name and is what the
// directory of the workspace and the other tables use, so it is taken once and
// renaming the project later does not move it. Two owners can have the same
// slug: the directory and the memory hang from the owner. A slug that an
// archived project of the same owner already has is not taken: that one comes
// back with the name, the conversations and the memory it had, because
// deleting a project archives it and does not let the name go.
func (s *Store) CreateProject(ctx context.Context, name string, owner Owner, createdBy string) (Project, error) {
	slug := naming.From(name)
	if slug == "" {
		return Project{}, errors.New("ese proyecto no tiene nombre")
	}
	if owner.Kind == "" || owner.ID == "" {
		return Project{}, errors.New("ese proyecto no tiene dueño")
	}
	var project Project
	err := s.db.QueryRowContext(ctx,
		"INSERT INTO chat.projects (slug, name, owner_kind, owner_id, created_by) VALUES ($1, $2, $3, $4, $5) "+
			"ON CONFLICT (owner_kind, owner_id, slug) DO UPDATE SET deleted_at = NULL, updated_at = goddard.now() "+
			"WHERE chat.projects.deleted_at IS NOT NULL "+
			"RETURNING "+projectColumns,
		slug, name, owner.Kind, owner.ID, createdBy).
		Scan(&project.ID, &project.Slug, &project.Name, &project.Owner.Kind, &project.Owner.ID, &project.CreatedBy)
	if errors.Is(err, sql.ErrNoRows) {
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

// DeleteProjects takes out every project of an owner. It is what deleting an
// organization does: the projects of a gone organization are nobody's, and
// nobody could reach them again. Marked, not dropped, like one project is.
func (s *Store) DeleteProjects(ctx context.Context, owner Owner) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE chat.projects SET deleted_at = goddard.now(), updated_at = goddard.now() "+
			"WHERE owner_kind = $1 AND owner_id = $2 AND deleted_at IS NULL", owner.Kind, owner.ID)
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
