package httpserver

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"task-service/internal/auth"
	"task-service/internal/cache"
	"task-service/internal/memstore"
	"task-service/internal/service"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestAPI(t *testing.T) {
	store := memstore.New()
	tokens := auth.NewManager("test-secret-value", time.Hour)
	accounts := service.NewAuth(store, tokens)
	teams := service.NewTeam(store)
	redisServer := miniredis.RunT(t)
	lists := cache.NewRedis(redis.NewClient(&redis.Options{Addr: redisServer.Addr()}))
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	tasks := service.NewTask(store, lists, time.Minute, logger)
	server := httptest.NewServer(New(accounts, teams, tasks, tokens, logger))
	defer server.Close()
	server.Client().CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	if resp := request(t, server, http.MethodGet, "/swagger/", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("swagger = %d %s", resp.StatusCode, resp.Body)
	}
	if resp := request(t, server, http.MethodGet, "/healthz", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("health = %d", resp.StatusCode)
	}
	root := request(t, server, http.MethodGet, "/", "", nil)
	if root.StatusCode != http.StatusFound || root.Header.Get("Location") != "/swagger/" {
		t.Fatalf("root = %d %s", root.StatusCode, root.Header.Get("Location"))
	}
	spec := request(t, server, http.MethodGet, "/openapi.yaml", "", nil)
	if spec.StatusCode != http.StatusOK || !strings.Contains(string(spec.Body), "/api/v1/tasks/{id}/history") {
		t.Fatalf("spec = %d", spec.StatusCode)
	}
	for _, path := range []string{
		"/healthz", "/api/v1/register", "/api/v1/login", "/api/v1/teams",
		"/api/v1/teams/{id}/invite", "/api/v1/teams/{team_id}/stats", "/api/v1/tasks",
		"/api/v1/tasks/{id}", "/api/v1/tasks/{id}/history", "/api/v1/tasks/{id}/comments",
	} {
		if !strings.Contains(string(openAPI), path+":") && !strings.Contains(string(openAPI), path) {
			t.Fatalf("openapi misses %s", path)
		}
	}

	owner := register(t, server, "owner@example.com", "Анна")
	if request(t, server, http.MethodPost, "/api/v1/register", "", map[string]string{"email": "owner@example.com", "password": "password123", "name": "Анна"}).StatusCode != http.StatusConflict {
		t.Fatal("expected conflict")
	}
	if request(t, server, http.MethodPost, "/api/v1/login", "", map[string]string{"email": "owner@example.com", "password": "nope-pass"}).StatusCode != http.StatusUnauthorized {
		t.Fatal("expected unauthorized")
	}
	ownerToken := login(t, server, "owner@example.com")
	if request(t, server, http.MethodGet, "/api/v1/teams", "", nil).StatusCode != http.StatusUnauthorized {
		t.Fatal("expected auth")
	}

	teamBody := request(t, server, http.MethodPost, "/api/v1/teams", ownerToken, map[string]string{"name": "Платформа"}).Body
	var team struct {
		ID int64 `json:"id"`
	}
	mustJSON(t, teamBody, &team)
	member := register(t, server, "member@example.com", "Борис")
	memberToken := login(t, server, "member@example.com")
	if request(t, server, http.MethodPost, "/api/v1/teams/"+itoa(team.ID)+"/invite", memberToken, map[string]any{"user_id": owner.ID, "role": "admin"}).StatusCode != http.StatusForbidden {
		t.Fatal("member cannot invite")
	}
	invite := request(t, server, http.MethodPost, "/api/v1/teams/"+itoa(team.ID)+"/invite", ownerToken, map[string]any{"email": member.Email, "role": "member"})
	if invite.StatusCode != http.StatusOK {
		t.Fatalf("invite = %d %s", invite.StatusCode, invite.Body)
	}
	teamsBody := request(t, server, http.MethodGet, "/api/v1/teams", memberToken, nil).Body
	if !strings.Contains(string(teamsBody), "Платформа") {
		t.Fatalf("teams = %s", teamsBody)
	}

	created := request(t, server, http.MethodPost, "/api/v1/tasks", ownerToken, map[string]any{"team_id": team.ID, "title": "Собрать отчет", "assignee_id": member.ID})
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d %s", created.StatusCode, created.Body)
	}
	var task struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}
	mustJSON(t, created.Body, &task)
	if request(t, server, http.MethodGet, "/api/v1/tasks", ownerToken, nil).StatusCode != http.StatusBadRequest {
		t.Fatal("team_id is required")
	}
	listed := request(t, server, http.MethodGet, "/api/v1/tasks?team_id="+itoa(team.ID)+"&status=todo&limit=20&offset=0", ownerToken, nil)
	if listed.StatusCode != http.StatusOK || !strings.Contains(string(listed.Body), "Собрать отчет") {
		t.Fatalf("list = %d %s", listed.StatusCode, listed.Body)
	}
	if request(t, server, http.MethodPut, "/api/v1/tasks/"+itoa(task.ID), memberToken, map[string]string{"status": "done"}).StatusCode != http.StatusBadRequest {
		t.Fatal("version is required")
	}
	updated := request(t, server, http.MethodPut, "/api/v1/tasks/"+itoa(task.ID), memberToken, map[string]any{"status": "done", "version": task.Version})
	if updated.StatusCode != http.StatusOK || !strings.Contains(string(updated.Body), `"status":"done"`) {
		t.Fatalf("update = %d %s", updated.StatusCode, updated.Body)
	}
	var saved struct {
		Version int64 `json:"version"`
	}
	mustJSON(t, updated.Body, &saved)
	if request(t, server, http.MethodPut, "/api/v1/tasks/"+itoa(task.ID), memberToken, map[string]any{"assignee_id": owner.ID, "version": saved.Version}).StatusCode != http.StatusForbidden {
		t.Fatal("assignee cannot reassign")
	}
	if request(t, server, http.MethodPut, "/api/v1/tasks/"+itoa(task.ID), ownerToken, map[string]any{"title": "устаревшее", "version": task.Version}).StatusCode != http.StatusConflict {
		t.Fatal("stale version must conflict")
	}
	history := request(t, server, http.MethodGet, "/api/v1/tasks/"+itoa(task.ID)+"/history", ownerToken, nil)
	if history.StatusCode != http.StatusOK || !strings.Contains(string(history.Body), "status") {
		t.Fatalf("history = %d %s", history.StatusCode, history.Body)
	}
	comment := request(t, server, http.MethodPost, "/api/v1/tasks/"+itoa(task.ID)+"/comments", ownerToken, map[string]string{"content": "проверено"})
	if comment.StatusCode != http.StatusCreated {
		t.Fatalf("comment = %d %s", comment.StatusCode, comment.Body)
	}
	comments := request(t, server, http.MethodGet, "/api/v1/tasks/"+itoa(task.ID)+"/comments", memberToken, nil)
	if comments.StatusCode != http.StatusOK || !strings.Contains(string(comments.Body), "проверено") {
		t.Fatalf("comments = %d %s", comments.StatusCode, comments.Body)
	}
	if request(t, server, http.MethodGet, "/api/v1/teams/"+itoa(team.ID)+"/stats", memberToken, nil).StatusCode != http.StatusForbidden {
		t.Fatal("member cannot read stats")
	}
	stats := request(t, server, http.MethodGet, "/api/v1/teams/"+itoa(team.ID)+"/stats", ownerToken, nil)
	if stats.StatusCode != http.StatusOK || !strings.Contains(string(stats.Body), `"comment_count":1`) {
		t.Fatalf("stats = %d %s", stats.StatusCode, stats.Body)
	}
	if request(t, server, http.MethodPost, "/api/v1/register", "", json.RawMessage(`{"email"`)).StatusCode != http.StatusBadRequest {
		t.Fatal("expected bad json")
	}
	if request(t, server, http.MethodPost, "/api/v1/register", "", map[string]string{"email": "bad", "password": "password123", "name": "Анна"}).StatusCode != http.StatusBadRequest {
		t.Fatal("expected bad email")
	}
	if request(t, server, http.MethodPost, "/api/v1/teams/"+itoa(team.ID)+"/invite", ownerToken, map[string]any{"email": member.Email, "role": "owner"}).StatusCode != http.StatusBadRequest {
		t.Fatal("expected owner role rejection")
	}
	if request(t, server, http.MethodGet, "/api/v1/tasks?team_id="+itoa(team.ID)+"&limit=1000", ownerToken, nil).StatusCode != http.StatusBadRequest {
		t.Fatal("expected limit error")
	}
	if request(t, server, http.MethodPut, "/api/v1/tasks/999999", ownerToken, map[string]any{"title": "нет", "version": 1}).StatusCode != http.StatusNotFound {
		t.Fatal("expected missing task")
	}
	if request(t, server, http.MethodPost, "/api/v1/tasks/"+itoa(task.ID)+"/comments", ownerToken, map[string]string{"content": " "}).StatusCode != http.StatusBadRequest {
		t.Fatal("expected empty comment")
	}
}

func TestLogsSkipSecrets(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	handler := logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/login", strings.NewReader(`{"password":"password123"}`))
	request.Header.Set("Authorization", "Bearer super-secret-token")
	handler.ServeHTTP(httptest.NewRecorder(), request)
	logged := buf.String()
	if strings.Contains(logged, "super-secret-token") || strings.Contains(logged, "password123") {
		t.Fatalf("log leaked secrets: %s", logged)
	}
	if !strings.Contains(logged, "/api/v1/login") {
		t.Fatalf("log = %s", logged)
	}
}

func TestRecoverer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := recoverer(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "внутренняя ошибка") {
		t.Fatalf("recorder = %d %s", recorder.Code, recorder.Body)
	}
}

type response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

func request(t *testing.T, server *httptest.Server, method, path, token string, payload any) response {
	t.Helper()
	var body io.Reader
	if payload != nil {
		raw, ok := payload.(json.RawMessage)
		if !ok {
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			raw = json.RawMessage(encoded)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, server.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{StatusCode: resp.StatusCode, Header: resp.Header, Body: raw}
}

func register(t *testing.T, server *httptest.Server, email, name string) struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
} {
	t.Helper()
	resp := request(t, server, http.MethodPost, "/api/v1/register", "", map[string]string{"email": email, "password": "password123", "name": name})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register %s = %d %s", email, resp.StatusCode, resp.Body)
	}
	var user struct {
		ID    int64  `json:"id"`
		Email string `json:"email"`
	}
	mustJSON(t, resp.Body, &user)
	return user
}

func login(t *testing.T, server *httptest.Server, email string) string {
	t.Helper()
	resp := request(t, server, http.MethodPost, "/api/v1/login", "", map[string]string{"email": email, "password": "password123"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login = %d %s", resp.StatusCode, resp.Body)
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	mustJSON(t, resp.Body, &token)
	return token.AccessToken
}

func mustJSON(t *testing.T, body []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
}

func itoa(value int64) string {
	return strconv.FormatInt(value, 10)
}
