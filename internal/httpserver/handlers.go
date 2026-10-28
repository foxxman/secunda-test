package httpserver

import (
	"net/http"
	"strings"

	"task-service/internal/domain"
	"task-service/internal/service"
)

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	user, err := h.auth.Register(r.Context(), body.Email, body.Password, body.Name)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, newUserResponse(user.ID, user.Email, user.Name, user.CreatedAt))
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	session, err := h.auth.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken: session.AccessToken,
		TokenType:   "Bearer",
		ExpiresAt:   session.ExpiresAt,
	})
}

func (h *Handler) createTeam(w http.ResponseWriter, r *http.Request) {
	actorID, err := h.actorID(r)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	team, err := h.teams.Create(r.Context(), actorID, body.Name)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, newTeamResponse(team))
}

func (h *Handler) listTeams(w http.ResponseWriter, r *http.Request) {
	actorID, err := h.actorID(r)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	teams, err := h.teams.List(r.Context(), actorID)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	items := make([]teamResponse, 0, len(teams))
	for _, team := range teams {
		items = append(items, newTeamResponse(team))
	}
	writeJSON(w, http.StatusOK, map[string]any{"teams": items})
}

func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	actorID, err := h.actorID(r)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	teamID, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	var body struct {
		UserID *int64 `json:"user_id"`
		Email  string `json:"email"`
		Role   string `json:"role"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	member, err := h.teams.Invite(r.Context(), actorID, teamID, service.Invite{
		UserID: body.UserID,
		Email:  body.Email,
		Role:   body.Role,
	})
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, memberResponse{TeamID: member.TeamID, UserID: member.UserID, Role: member.Role})
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	actorID, err := h.actorID(r)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	teamID, err := pathID(r, "teamID")
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	stats, err := h.teams.Stats(r.Context(), actorID, teamID)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	if stats.TasksByStatus == nil {
		stats.TasksByStatus = domain.EmptyStats().TasksByStatus
	}
	if stats.TopAssignees == nil {
		stats.TopAssignees = []domain.TopAssignee{}
	}
	writeJSON(w, http.StatusOK, statsResponse{
		TeamID:          teamID,
		TasksByStatus:   stats.TasksByStatus,
		TopAssignees:    stats.TopAssignees,
		AvgCloseSeconds: stats.AvgCloseSeconds,
		CommentCount:    stats.CommentCount,
	})
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	actorID, err := h.actorID(r)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	var body struct {
		TeamID      int64  `json:"team_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
		AssigneeID  *int64 `json:"assignee_id"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	task, err := h.tasks.Create(r.Context(), actorID, service.TaskInput{
		TeamID:      body.TeamID,
		Title:       body.Title,
		Description: body.Description,
		Status:      body.Status,
		AssigneeID:  body.AssigneeID,
	})
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, newTaskResponse(task))
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	actorID, err := h.actorID(r)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	teamID, err := queryInt64(r, "team_id")
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	if teamID == nil {
		writeErr(w, r, h.log, domain.Invalid("team_id обязателен"))
		return
	}
	assigneeID, err := queryInt64(r, "assignee_id")
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	limit, limitSet, err := queryInt(r, "limit")
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	offset, _, err := queryInt(r, "offset")
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	if !limitSet || limit == 0 {
		limit = 20
	}
	filter := domain.TaskFilter{
		TeamID:     *teamID,
		Status:     r.URL.Query().Get("status"),
		AssigneeID: assigneeID,
		Limit:      limit,
		Offset:     offset,
	}
	tasks, err := h.tasks.List(r.Context(), actorID, filter)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, newTaskList(tasks, filter.Limit, filter.Offset))
}

func (h *Handler) updateTask(w http.ResponseWriter, r *http.Request) {
	actorID, err := h.actorID(r)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	taskID, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	var body updateTaskBody
	if err := decode(r, &body); err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	patch, err := taskPatch(body)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	task, err := h.tasks.Update(r.Context(), actorID, taskID, patch)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, newTaskResponse(task))
}

func (h *Handler) taskHistory(w http.ResponseWriter, r *http.Request) {
	actorID, err := h.actorID(r)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	taskID, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	entries, err := h.tasks.History(r.Context(), actorID, taskID)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	items := make([]historyResponse, 0, len(entries))
	for _, entry := range entries {
		items = append(items, historyResponse{
			ID: entry.ID, TaskID: entry.TaskID, ChangedBy: entry.ChangedBy,
			Changes: entry.Changes, CreatedAt: entry.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": items})
}

func (h *Handler) addComment(w http.ResponseWriter, r *http.Request) {
	actorID, err := h.actorID(r)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	taskID, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	comment, err := h.tasks.AddComment(r.Context(), actorID, taskID, body.Content)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, commentResponse{
		ID: comment.ID, TaskID: comment.TaskID, UserID: comment.UserID,
		Content: comment.Content, CreatedAt: comment.CreatedAt,
	})
}

func (h *Handler) listComments(w http.ResponseWriter, r *http.Request) {
	actorID, err := h.actorID(r)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	taskID, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	comments, err := h.tasks.Comments(r.Context(), actorID, taskID)
	if err != nil {
		writeErr(w, r, h.log, err)
		return
	}
	items := make([]commentResponse, 0, len(comments))
	for _, comment := range comments {
		items = append(items, commentResponse{
			ID: comment.ID, TaskID: comment.TaskID, UserID: comment.UserID,
			Content: comment.Content, CreatedAt: comment.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": items})
}

func (h *Handler) actorID(r *http.Request) (int64, error) {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return 0, domain.ErrUnauthorized
	}
	return h.tokens.Parse(strings.TrimSpace(strings.TrimPrefix(header, prefix)))
}
