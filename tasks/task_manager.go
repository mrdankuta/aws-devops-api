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
	ID             string
	Name           string
	AWSAccounts    []string
	Service        string
	Command        string
	Schedule       cron.Schedule
	ScheduleString string
	SlackChannel   string
	Execute        func() (string, error)
	cronEntryID    int
}

func NewTaskManager(taskConfigs *[]config.TaskConfig, authModule *auth.AuthModule) *TaskManager {
	tm := &TaskManager{
		tasks:      make(map[string]Task),
		authModule: authModule,
		cron:       cron.New(),
	}

	fmt.Printf("Initializing TaskManager with %d task configs\n", len(*taskConfigs))

	for _, cfg := range *taskConfigs {
		schedule, err := cron.ParseStandard(cfg.Schedule)
		if err != nil {
			fmt.Printf("Error parsing schedule for task %s: %v\n", cfg.Name, err)
			continue
		}

		taskID := uuid.New().String()
		task := Task{
			ID:             taskID,
			Name:           cfg.Name,
			AWSAccounts:    cfg.AWSAccounts,
			Service:        cfg.Service,
			Command:        cfg.Command,
			Schedule:       schedule,
			ScheduleString: cfg.Schedule,
			SlackChannel:   cfg.SlackChannel,
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
		fmt.Printf("Added task: ID=%s, Name=%s\n", taskID, task.Name)
		tm.cron.Schedule(schedule, cron.FuncJob(func() {
			tm.ExecuteTask(taskID)
		}))
	}

	fmt.Printf("TaskManager initialized with %d tasks\n", len(tm.tasks))
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
	for id, task := range tm.tasks {
		fmt.Printf("Task found: ID=%s, Name=%s\n", id, task.Name)
		tasks = append(tasks, task)
	}
	fmt.Printf("Total tasks: %d\n", len(tasks))
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

func (tm *TaskManager) CreateTask(cfg config.TaskConfig) (Task, error) {
	schedule, err := cron.ParseStandard(cfg.Schedule)
	if err != nil {
		return Task{}, fmt.Errorf("error parsing schedule: %w", err)
	}

	taskID := uuid.New().String()
	task := Task{
		ID:             taskID,
		Name:           cfg.Name,
		AWSAccounts:    cfg.AWSAccounts,
		Service:        cfg.Service,
		Command:        cfg.Command,
		Schedule:       schedule,
		ScheduleString: cfg.Schedule,
		SlackChannel:   cfg.SlackChannel,
	}

	switch cfg.Service {
	case "s3":
		task.Execute = s3.NewCommand(cfg.Command, cfg.AWSAccounts, tm.authModule)
	case "iam":
		task.Execute = iam.NewCommand(cfg.Command, cfg.AWSAccounts, tm.authModule)
	default:
		return Task{}, fmt.Errorf("unknown service: %s", cfg.Service)
	}

	tm.tasks[taskID] = task
	tm.cron.Schedule(schedule, cron.FuncJob(func() {
		tm.ExecuteTask(taskID)
	}))

	return task, nil
}

// Update the UpdateTask method to use cron.EntryID
func (tm *TaskManager) UpdateTask(id string, updatedTask Task) error {
	oldTask, exists := tm.tasks[id]
	if !exists {
		return fmt.Errorf("task with ID %s not found", id)
	}

	// Remove the old cron job
	tm.cron.Remove(cron.EntryID(oldTask.cronEntryID))

	// Schedule the new job
	entryID, err := tm.cron.AddFunc(updatedTask.ScheduleString, func() {
		tm.ExecuteTask(id)
	})
	if err != nil {
		return fmt.Errorf("failed to schedule updated task: %w", err)
	}

	// Update the task in the map
	updatedTask.cronEntryID = int(entryID)
	tm.tasks[id] = updatedTask
	return nil
}

// Update the DeleteTask method to use cron.EntryID
func (tm *TaskManager) DeleteTask(id string) error {
	task, exists := tm.tasks[id]
	if !exists {
		return fmt.Errorf("task with ID %s not found", id)
	}

	// Remove the cron job
	tm.cron.Remove(cron.EntryID(task.cronEntryID))

	// Remove the task from the map
	delete(tm.tasks, id)
	return nil
}
