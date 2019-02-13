package main

import (
    "os"
    "os/signal"
    "syscall"
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

func main() {
    logger, _ = zap.NewDevelopment()

    sigs := make(chan os.Signal, 1)
    done := make(chan bool, 1)
    signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

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

    go func(job *kube.Job) {
        <-sigs

        if kube.Exists(job) {
            logger.Info("Cleaning up")
            kube.DeleteJob(job)
        }

        done <- true
    }(job)

    err = kube.CreateJob(job)
    if err != nil {
        logger.Error(err.Error())
        os.Exit(1)
    }

    kube.TailLog(job)
    isComplete := kube.IsComplete(job)

    syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
    <-done

    if !isComplete {
        logger.Error("The job cannot complete!")
        os.Exit(1)
    }

    logger.Info("COMPLETE")
}
