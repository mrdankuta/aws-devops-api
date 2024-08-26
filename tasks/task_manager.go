package tasks

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mrdankuta/aws-devops-api/auth"
	"github.com/mrdankuta/aws-devops-api/config"
	"github.com/mrdankuta/aws-devops-api/services/iam"
	"github.com/mrdankuta/aws-devops-api/services/s3"
	"github.com/robfig/cron/v3"
)

type TaskManager struct {
	tasks      map[string]Task
	authModule *auth.AuthModule
	cron       *cron.Cron
}

type Task struct {
	ID           string
	Name         string
	AWSAccounts  []string
	Service      string
	Command      string
	Schedule     cron.Schedule
	SlackChannel string
	Execute      func() (string, error)
}

func NewTaskManager(taskConfigs *[]config.TaskConfig, authModule *auth.AuthModule) *TaskManager {
	tm := &TaskManager{
		tasks:      make(map[string]Task),
		authModule: authModule,
		cron:       cron.New(),
	}

	for _, cfg := range *taskConfigs {
		schedule, err := cron.ParseStandard(cfg.Schedule)
		if err != nil {
			fmt.Printf("Error parsing schedule for task %s: %v\n", cfg.Name, err)
			continue
		}

		taskID := uuid.New().String()
		task := Task{
			ID:           taskID,
			Name:         cfg.Name,
			AWSAccounts:  cfg.AWSAccounts,
			Service:      cfg.Service,
			Command:      cfg.Command,
			Schedule:     schedule,
			SlackChannel: cfg.SlackChannel,
		}

		switch cfg.Service {
		case "s3":
			task.Execute = s3.NewCommand(cfg.Command, cfg.AWSAccounts, authModule)
		case "iam":
			task.Execute = iam.NewCommand(cfg.Command, cfg.AWSAccounts, authModule)
		default:
			fmt.Printf("Unknown service for task %s: %s\n", cfg.Name, cfg.Service)
			continue
		}

		tm.tasks[taskID] = task
		tm.cron.Schedule(schedule, cron.FuncJob(func() {
			tm.ExecuteTask(taskID)
		}))
	}

	tm.cron.Start()
	return tm
}

func (tm *TaskManager) GetDueTasks() []Task {
	var dueTasks []Task
	now := time.Now()
	for _, task := range tm.tasks {
		if task.Schedule.Next(now).Sub(now) < time.Minute {
			dueTasks = append(dueTasks, task)
		}
	}
	return dueTasks
}

func (tm *TaskManager) GetAllTasks() []Task {
	tasks := make([]Task, 0, len(tm.tasks))
	for _, task := range tm.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

func (tm *TaskManager) GetTask(id string) (Task, bool) {
	task, exists := tm.tasks[id]
	return task, exists
}

func (tm *TaskManager) ExecuteTask(id string) (string, error) {
	task, exists := tm.tasks[id]
	if !exists {
		return "", fmt.Errorf("task with ID %s not found", id)
	}

	result, err := task.Execute()
	if err != nil {
		fmt.Printf("Error executing task %s: %v\n", task.Name, err)
		return "", err
	}

	fmt.Printf("Task %s executed successfully: %s\n", task.Name, result)
	return result, nil
}
