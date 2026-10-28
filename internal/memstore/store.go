package memstore

import (
	"context"
	"strings"
	"sync"
	"time"

	"task-service/internal/domain"
)

type Store struct {
	mu       sync.Mutex
	users    map[int64]domain.User
	emails   map[string]int64
	teams    map[int64]domain.Team
	members  map[int64]map[int64]domain.Member
	tasks    map[int64]domain.Task
	history  []domain.HistoryEntry
	comments []domain.Comment
	nextID   int64
}

func New() *Store {
	return &Store{
		users:   map[int64]domain.User{},
		emails:  map[string]int64{},
		teams:   map[int64]domain.Team{},
		members: map[int64]map[int64]domain.Member{},
		tasks:   map[int64]domain.Task{},
	}
}

func (s *Store) CreateUser(_ context.Context, user domain.User) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.emails[user.Email]; ok {
		return domain.User{}, domain.ErrConflict
	}
	user.ID = s.id()
	s.users[user.ID] = user
	s.emails[user.Email] = user.ID
	return user, nil
}

func (s *Store) UserByEmail(_ context.Context, email string) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.emails[strings.ToLower(email)]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return s.users[id], nil
}

func (s *Store) UserByID(_ context.Context, id int64) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return user, nil
}

func (s *Store) CreateTeam(_ context.Context, name string, ownerID int64, now time.Time) (domain.Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[ownerID]; !ok {
		return domain.Team{}, domain.ErrNotFound
	}
	team := domain.Team{
		ID:        s.id(),
		Name:      name,
		CreatedBy: ownerID,
		Role:      domain.RoleOwner,
		CreatedAt: now.UTC(),
	}
	s.teams[team.ID] = team
	s.members[team.ID] = map[int64]domain.Member{
		ownerID: {TeamID: team.ID, UserID: ownerID, Role: domain.RoleOwner, CreatedAt: now.UTC()},
	}
	return team, nil
}

func (s *Store) ListTeams(_ context.Context, userID int64) ([]domain.Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	teams := make([]domain.Team, 0)
	for id, team := range s.teams {
		member, ok := s.members[id][userID]
		if !ok && team.CreatedBy != userID {
			continue
		}
		team.Role = domain.RoleOwner
		if ok {
			team.Role = member.Role
		}
		teams = append(teams, team)
	}
	sortTeams(teams)
	return teams, nil
}

func (s *Store) TeamByID(_ context.Context, id int64) (domain.Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	team, ok := s.teams[id]
	if !ok {
		return domain.Team{}, domain.ErrNotFound
	}
	return team, nil
}

func (s *Store) TeamMember(_ context.Context, teamID, userID int64) (domain.Member, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	member, ok := s.members[teamID][userID]
	if !ok {
		return domain.Member{}, domain.ErrNotFound
	}
	return member, nil
}

func (s *Store) AddMember(_ context.Context, member domain.Member) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.teams[member.TeamID]; !ok {
		return domain.ErrNotFound
	}
	if _, ok := s.members[member.TeamID][member.UserID]; ok {
		return domain.ErrConflict
	}
	if s.members[member.TeamID] == nil {
		s.members[member.TeamID] = map[int64]domain.Member{}
	}
	s.members[member.TeamID][member.UserID] = member
	return nil
}

func (s *Store) UpdateMemberRole(_ context.Context, teamID, userID int64, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	member, ok := s.members[teamID][userID]
	if !ok || member.Role == domain.RoleOwner {
		return domain.ErrNotFound
	}
	member.Role = role
	s.members[teamID][userID] = member
	return nil
}

func (s *Store) CreateTask(_ context.Context, task domain.Task, changes domain.Changes) (domain.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task.ID = s.id()
	s.tasks[task.ID] = task
	s.history = append(s.history, domain.HistoryEntry{
		ID:        s.id(),
		TaskID:    task.ID,
		ChangedBy: task.CreatedBy,
		Changes:   changes,
		CreatedAt: task.CreatedAt,
	})
	return task, nil
}

func (s *Store) ListTasks(_ context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	matched := make([]domain.Task, 0)
	for _, task := range s.tasks {
		if task.TeamID != filter.TeamID {
			continue
		}
		if filter.Status != "" && task.Status != filter.Status {
			continue
		}
		if filter.AssigneeID != nil && (task.AssigneeID == nil || *task.AssigneeID != *filter.AssigneeID) {
			continue
		}
		matched = append(matched, task)
	}
	sortTasksDesc(matched)
	if filter.Offset >= len(matched) {
		return []domain.Task{}, nil
	}
	matched = matched[filter.Offset:]
	if filter.Limit < len(matched) {
		matched = matched[:filter.Limit]
	}
	return matched, nil
}

func (s *Store) TaskByID(_ context.Context, id int64) (domain.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return domain.Task{}, domain.ErrNotFound
	}
	return task, nil
}

func (s *Store) UpdateTask(_ context.Context, id, actorID, expectedVersion int64, now time.Time, apply func(domain.Task) (domain.Task, domain.Changes, error)) (domain.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.tasks[id]
	if !ok {
		return domain.Task{}, domain.ErrNotFound
	}
	if current.Version != expectedVersion {
		return domain.Task{}, domain.ErrConflict
	}
	next, changes, err := apply(current)
	if err != nil {
		return domain.Task{}, err
	}
	if len(changes) == 0 {
		return current, nil
	}
	next.ID = current.ID
	next.TeamID = current.TeamID
	next.CreatedBy = current.CreatedBy
	next.CreatedAt = current.CreatedAt
	next.Version = expectedVersion + 1
	next.UpdatedAt = now.UTC()
	s.tasks[id] = next
	s.history = append(s.history, domain.HistoryEntry{
		ID:        s.id(),
		TaskID:    id,
		ChangedBy: actorID,
		Changes:   changes,
		CreatedAt: next.UpdatedAt,
	})
	return next, nil
}

func (s *Store) Comments(_ context.Context, taskID int64) ([]domain.Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Comment, 0)
	for _, comment := range s.comments {
		if comment.TaskID == taskID {
			out = append(out, comment)
		}
	}
	return out, nil
}

func (s *Store) AddComment(_ context.Context, comment domain.Comment) (domain.Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[comment.TaskID]; !ok {
		return domain.Comment{}, domain.ErrNotFound
	}
	comment.ID = s.id()
	s.comments = append(s.comments, comment)
	return comment, nil
}

func (s *Store) History(_ context.Context, taskID int64) ([]domain.HistoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.HistoryEntry, 0)
	for _, entry := range s.history {
		if entry.TaskID == taskID {
			out = append(out, entry)
		}
	}
	return out, nil
}

func (s *Store) Stats(_ context.Context, teamID int64, since time.Time) (domain.Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks := make([]domain.Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}
	comments := append([]domain.Comment(nil), s.comments...)
	return domain.ComputeStats(tasks, comments, teamID, since), nil
}

func (s *Store) SetTask(task domain.Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.ID] = task
}

func (s *Store) id() int64 {
	s.nextID++
	return s.nextID
}

func sortTeams(teams []domain.Team) {
	for i := 1; i < len(teams); i++ {
		for j := i; j > 0 && teams[j].ID < teams[j-1].ID; j-- {
			teams[j], teams[j-1] = teams[j-1], teams[j]
		}
	}
}

func sortTasksDesc(tasks []domain.Task) {
	for i := 1; i < len(tasks); i++ {
		for j := i; j > 0 && tasks[j].ID > tasks[j-1].ID; j-- {
			tasks[j], tasks[j-1] = tasks[j-1], tasks[j]
		}
	}
}
