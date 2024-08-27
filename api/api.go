package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/mrdankuta/aws-devops-api/auth"
	"github.com/mrdankuta/aws-devops-api/config"
	"github.com/mrdankuta/aws-devops-api/tasks"
	"golang.org/x/oauth2"
)

type API struct {
	config      *config.Config
	authModule  *auth.AuthModule
	taskManager *tasks.TaskManager
}

func NewAPI(cfg *config.Config, authModule *auth.AuthModule, taskManager *tasks.TaskManager) *API {
	return &API{
		config:      cfg,
		authModule:  authModule,
		taskManager: taskManager,
	}
}

func (api *API) SetupRoutes(router *mux.Router) {

	// TODO: Remove this before production
	if bypassAuth {
		router.HandleFunc("/dev/token", api.generateTestToken).Methods("GET")
	}

	// Auth routes (unprotected)
	router.HandleFunc("/auth/login", api.authModule.StartOIDCFlow).Methods("GET")
	router.HandleFunc("/auth/callback", api.authModule.HandleCallback).Methods("GET")

	// Protected API routes
	apiRouter := router.PathPrefix("/api").Subrouter()
	apiRouter.Use(api.authMiddleware)

	apiRouter.HandleFunc("/tasks", api.GetTasks).Methods("GET")
	apiRouter.HandleFunc("/tasks", api.CreateTask).Methods("POST")
	apiRouter.HandleFunc("/tasks/{id}", api.UpdateTask).Methods("PUT")
	apiRouter.HandleFunc("/tasks/{id}", api.DeleteTask).Methods("DELETE")
	apiRouter.HandleFunc("/tasks/{id}/execute", api.ExecuteTask).Methods("POST")
	apiRouter.HandleFunc("/settings", api.GetSettings).Methods("GET")
	apiRouter.HandleFunc("/settings", api.UpdateSettings).Methods("PUT")
}

var bypassAuth bool = true // Set to true for testing. Remove this before production.

func (api *API) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Remove this before production
		if bypassAuth {
			next.ServeHTTP(w, r)
			return
		}
		// Check if the user is authenticated
		// TODO: This is a simplified check. Implement proper token validation.
		if !api.authModule.HasValidToken(r.Header.Get("Authorization")) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (api *API) GetTasks(w http.ResponseWriter, r *http.Request) {
	tasks := api.taskManager.GetAllTasks()
	json.NewEncoder(w).Encode(tasks)
}

func (api *API) CreateTask(w http.ResponseWriter, r *http.Request) {
	var taskConfig config.TaskConfig
	if err := json.NewDecoder(r.Body).Decode(&taskConfig); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	task, err := api.taskManager.CreateTask(taskConfig)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

func (api *API) UpdateTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["id"]

	var updatedTask tasks.Task
	if err := json.NewDecoder(r.Body).Decode(&updatedTask); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := api.taskManager.UpdateTask(taskID, updatedTask); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (api *API) DeleteTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["id"]

	if err := api.taskManager.DeleteTask(taskID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (api *API) ExecuteTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["id"]

	result, err := api.taskManager.ExecuteTask(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"result": result})
}

func (api *API) GetSettings(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(api.config)
}

func (api *API) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var updatedConfig config.Config
	if err := json.NewDecoder(r.Body).Decode(&updatedConfig); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Update the configuration
	api.config.OIDC = updatedConfig.OIDC
	api.config.Slack = updatedConfig.Slack

	// Save the updated configuration to file
	if err := config.Save("config.yaml", api.config); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// TODO: Remove this before production
func (api *API) generateTestToken(w http.ResponseWriter, r *http.Request) {
	// Generate a dummy token for testing
	token := &oauth2.Token{
		AccessToken: "test_access_token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(1 * time.Hour),
	}
	json.NewEncoder(w).Encode(token)
}
