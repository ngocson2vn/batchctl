package main

import (
	"os"
	"fmt"
	"strconv"
	"go.uber.org/zap"
	"github.com/FiNCDeveloper/batchctl/libs/kube"
)

var logger *zap.Logger

func getCommand() []string {
	command := []string{}
	argLen := len(os.Args)
	if argLen >= 2 {
		for _, c := range os.Args[1:] {
			command = append(command, fmt.Sprintf("\"%s\"", c))
		}
	}

	return command
}

func getCPUAndMemory() (int64 , int64) {
	var cpu int64 = 2
	var mem int64 = 128
	var err error = nil

	if cpu, err = strconv.ParseInt(os.Getenv("CPU_NUM"), 10, 64); err != nil {
		logger.Info("Using the default number of vCPUs: 2")
		cpu = 2
	}

	if mem, err = strconv.ParseInt(os.Getenv("MEMORY_MB"), 10, 64); err != nil {
		logger.Info("Using the default number of MB of memory: 128MB")
		mem = 128
	}

	return cpu, mem
}

func main() {
	logger, _ = zap.NewDevelopment()

	command := getCommand()
	if len(command) == 0 {
		logger.Error("Batch command is not specified.")
		os.Exit(1)
	}

	retryCount := 60
	var err error = nil
	if len(os.Getenv("RETRY_COUNT")) > 0 {
		retryCount, err = strconv.Atoi(os.Getenv("RETRY_COUNT"))
		if err != nil {
			logger.Error(err.Error())
			os.Exit(1)
		}
	}

	job := &kube.Job {
		ServiceName:    os.Getenv("SERVICE_NAME"),
		JobName:        os.Getenv("JOB_NAME"),
		RetryCount:     retryCount,
		Command:        command,
		Logger:         logger,
	}

	err = kube.CreateJob(job)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	err = kube.TailLog(job)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	// Get job status
	isComplete, err := kube.IsComplete(job)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	// Clean up
	logger.Info("Cleaning...")
	err = kube.DeleteJob(job)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	if !isComplete {
		logger.Error("The job cannot complete!")
		os.Exit(1)
	}

	logger.Info("COMPLETE")
}
