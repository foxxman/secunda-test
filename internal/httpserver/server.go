package httpserver

import (
	"log/slog"
	"net/http"

	"task-service/internal/auth"
	"task-service/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/swaggest/swgui/v5emb"
)

type Handler struct {
	auth   *service.Auth
	teams  *service.Team
	tasks  *service.Task
	tokens *auth.Manager
	log    *slog.Logger
}

func New(authService *service.Auth, teams *service.Team, tasks *service.Task, tokens *auth.Manager, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{auth: authService, teams: teams, tasks: tasks, tokens: tokens, log: logger}
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(logging(logger))
	router.Use(recoverer(logger))
	router.Use(limitBody)

	router.Get("/healthz", h.health)
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger/", http.StatusFound)
	})
	router.Get("/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(openAPI)
	})
	router.Mount("/swagger", v5emb.New("Сервис управления задачами", "/openapi.yaml", "/swagger"))

	router.Route("/api/v1", func(api chi.Router) {
		api.Post("/register", h.register)
		api.Post("/login", h.login)
		api.Group(func(private chi.Router) {
			private.Post("/teams", h.createTeam)
			private.Get("/teams", h.listTeams)
			private.Post("/teams/{id}/invite", h.invite)
			private.Get("/teams/{teamID}/stats", h.stats)
			private.Post("/tasks", h.createTask)
			private.Get("/tasks", h.listTasks)
			private.Put("/tasks/{id}", h.updateTask)
			private.Get("/tasks/{id}/history", h.taskHistory)
			private.Post("/tasks/{id}/comments", h.addComment)
			private.Get("/tasks/{id}/comments", h.listComments)
		})
	})
	return router
}
