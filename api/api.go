package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mrdankuta/aws-devops-api/auth"
	"github.com/mrdankuta/aws-devops-api/config"
	"github.com/mrdankuta/aws-devops-api/tasks"
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
	router.HandleFunc("/api/tasks", api.GetTasks).Methods("GET")
	router.HandleFunc("/api/tasks", api.CreateTask).Methods("POST")
	router.HandleFunc("/api/tasks/{id}", api.UpdateTask).Methods("PUT")
	router.HandleFunc("/api/tasks/{id}", api.DeleteTask).Methods("DELETE")
	router.HandleFunc("/api/tasks/{id}/execute", api.ExecuteTask).Methods("POST")
	router.HandleFunc("/api/settings", api.GetSettings).Methods("GET")
	router.HandleFunc("/api/settings", api.UpdateSettings).Methods("PUT")
}

func (api *API) GetTasks(w http.ResponseWriter, r *http.Request) {
	tasks := api.taskManager.GetAllTasks()
	json.NewEncoder(w).Encode(tasks)
}

func (api *API) CreateTask(w http.ResponseWriter, r *http.Request) {
	var task config.TaskConfig
	json.NewDecoder(r.Body).Decode(&task)
	// Add logic to create a new task
	w.WriteHeader(http.StatusCreated)
}

func (api *API) UpdateTask(w http.ResponseWriter, r *http.Request) {
	// vars := mux.Vars(r)
	// taskID := vars["id"]
	var updatedTask config.TaskConfig
	json.NewDecoder(r.Body).Decode(&updatedTask)
	// Add logic to update the task
	w.WriteHeader(http.StatusOK)
}

func (api *API) DeleteTask(w http.ResponseWriter, r *http.Request) {
	// vars := mux.Vars(r)
	// taskID := vars["id"]
	// Add logic to delete the task
	w.WriteHeader(http.StatusNoContent)
}

func (api *API) ExecuteTask(w http.ResponseWriter, r *http.Request) {
	// vars := mux.Vars(r)
	// taskID := vars["id"]
	// Add logic to execute the task
	w.WriteHeader(http.StatusOK)
}

func (api *API) GetSettings(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(api.config)
}

func (api *API) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var updatedConfig config.Config
	json.NewDecoder(r.Body).Decode(&updatedConfig)
	// Add logic to update the config
	w.WriteHeader(http.StatusOK)
}
