package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/mrdankuta/aws-devops-api/api"
	"github.com/mrdankuta/aws-devops-api/auth"
	"github.com/mrdankuta/aws-devops-api/config"
	"github.com/mrdankuta/aws-devops-api/slack"
	"github.com/mrdankuta/aws-devops-api/tasks"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

func init() {
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	log.SetLevel(logrus.DebugLevel)
}

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
	fmt.Printf("TaskManager created with %d tasks\n", len(taskManager.GetAllTasks()))

	// Initialize Slack client
	slackClient := slack.NewClient(cfg.Slack.Token)

	// Initialize API
	api := api.NewAPI(cfg, authModule, taskManager)

	// Set up HTTP router
	router := mux.NewRouter()
	api.SetupRoutes(router)
	// authModule.SetupRoutes(router)
	// apiHandler.SetupRoutes(router)

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

	log.Info("Starting application")

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
			log.WithFields(logrus.Fields{
				"taskID":   t.ID,
				"taskName": t.Name,
			}).Debug("Executing task")

			result, err := t.Execute()
			if err != nil {
				log.WithFields(logrus.Fields{
					"taskID":   t.ID,
					"taskName": t.Name,
					"error":    err,
				}).Error("Error executing task")
				return
			}

			log.WithFields(logrus.Fields{
				"taskID":   t.ID,
				"taskName": t.Name,
				"result":   result,
			}).Info("Task executed successfully")

			if err := sc.PostMessage(t.SlackChannel, result); err != nil {
				log.WithFields(logrus.Fields{
					"taskID":       t.ID,
					"taskName":     t.Name,
					"slackChannel": t.SlackChannel,
					"error":        err,
				}).Error("Error posting to Slack")
			}
		}(task)
	}
}
