package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/mrdankuta/aws-devops-api/auth"
	"github.com/mrdankuta/aws-devops-api/config"
	"github.com/mrdankuta/aws-devops-api/slack"
	"github.com/mrdankuta/aws-devops-api/tasks"
)

func main() {
	// Load configuration
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize authentication module
	authModule, err := auth.NewAuthModule(&cfg.OIDC)
	if err != nil {
		log.Fatalf("Failed to create auth module: %v", err)
	}

	// Initialize task manager
	taskManager := tasks.NewTaskManager(&cfg.Tasks, authModule)

	// Initialize Slack client
	slackClient := slack.NewClient(cfg.Slack.Token)

	// Set up HTTP router
	router := mux.NewRouter()
	authModule.SetupRoutes(router)

	// Set up HTTP server
	srv := &http.Server{
		Addr:    ":9090",
		Handler: router,
	}

	// Start HTTP server
	go func() {
		log.Printf("Starting HTTP server on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Start task execution loop
	go func() {
		for {
			executeTasks(taskManager, slackClient)
			time.Sleep(1 * time.Minute) // Check every minute
		}
	}()

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting")
}

func executeTasks(tm *tasks.TaskManager, sc *slack.Client) {
	for _, task := range tm.GetDueTasks() {
		go func(t tasks.Task) {
			result, err := t.Execute()
			if err != nil {
				log.Printf("Error executing task %s: %v", t.Name, err)
				return
			}
			if err := sc.PostMessage(t.SlackChannel, result); err != nil {
				log.Printf("Error posting to Slack for task %s: %v", t.Name, err)
			}
		}(task)
	}
}
