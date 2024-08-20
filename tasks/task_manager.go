package tasks

import (
	"fmt"
	"time"

	"github.com/mrdankuta/aws-devops-api/auth"
	"github.com/mrdankuta/aws-devops-api/services/iam"
	"github.com/mrdankuta/aws-devops-api/services/s3"
	"github.com/robfig/cron/v3"
)

type TaskManager struct {
	tasks      []Task
	authModule *auth.AuthModule
	cron       *cron.Cron
}

type Task struct {
	Name         string
	AWSAccounts  []string
	Service      string
	Command      string
	Schedule     cron.Schedule
	SlackChannel string
	Execute      func() (string, error)
}

func NewTaskManager(taskConfigs []TaskConfig, authModule *auth.AuthModule) *TaskManager {
	tm := &TaskManager{
		authModule: authModule,
		cron:       cron.New(),
	}

	for _, cfg := range taskConfigs {
		schedule, err := cron.ParseStandard(cfg.Schedule)
		if err != nil {
			fmt.Printf("Error parsing schedule for task %s: %v\n", cfg.Name, err)
			continue
		}

		task := Task{
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

		tm.tasks = append(tm.tasks, task)
		tm.cron.Schedule(schedule, cron.FuncJob(func() {
			result, err := task.Execute()
			if err != nil {
				fmt.Printf("Error executing task %s: %v\n", task.Name, err)
			} else {
				fmt.Printf("Task %s executed successfully: %s\n", task.Name, result)
			}
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
