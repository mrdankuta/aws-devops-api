package main

import (
	"log"
	"time"

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
	authModule := auth.NewAuthModule(cfg.OIDC)

	// Initialize task manager
	taskManager := tasks.NewTaskManager(cfg.Tasks, authModule)

	// Initialize Slack client
	slackClient := slack.NewClient(cfg.Slack.Token)

	// Start task execution loop
	for {
		executeTasks(taskManager, slackClient)
		time.Sleep(1 * time.Minute) // Check every minute
	}
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
