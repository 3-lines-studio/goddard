package app

import (
	"context"
	"fmt"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/schedule"
)

// runTask answers a task of the agenda. It is the runner the scheduler is
// given: the prompt runs in the conversation the task owns inside its project,
// which is a thread of its own and not the one the task was asked for from, so
// what a task remembers is what it answered before and nothing else.
//
// What comes back is what the agent said, and the run the scheduler writes
// down keeps it. The thread keeps it too, which is where the web reads it.
func (s *Service) runTask(ctx context.Context, task schedule.Task) (string, error) {
	conversation, err := s.taskThread(ctx, task)
	if err != nil {
		return "", err
	}
	if _, err := s.write(ctx, conversation.ID, map[string]any{"event": "user", "text": task.Prompt}); err != nil {
		return "", err
	}
	return s.answer(ctx, conversation, task.Prompt)
}

// taskThread is the conversation a task runs in: one per task, named after it,
// made the first time the task fires. It is born read only, like every thread
// that is not the web's: the task writes it and the web watches.
func (s *Service) taskThread(ctx context.Context, task schedule.Task) (chat.Conversation, error) {
	projects, err := s.Chat.Projects(ctx)
	if err != nil {
		return chat.Conversation{}, err
	}
	project, found := chat.Project{}, false
	for _, candidate := range projects {
		if candidate.Slug == task.Project {
			project, found = candidate, true
			break
		}
	}
	if !found {
		return chat.Conversation{}, fmt.Errorf("el proyecto %q no existe", task.Project)
	}
	conversations, err := s.Chat.Conversations(ctx, project.ID)
	if err != nil {
		return chat.Conversation{}, err
	}
	for _, conversation := range conversations {
		if conversation.Title == task.Name {
			return conversation, nil
		}
	}
	return s.Chat.CreateConversation(ctx, project.ID, task.Name, chat.SourceSchedule, task.UserID)
}
