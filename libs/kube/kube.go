package kube

import (
    "fmt"
    "bufio"
    "os/exec"
    "strings"
    "bytes"
    "io"
    "os"
    "time"

    "go.uber.org/zap"
)

type Job struct {
    ServiceName    string
    JobName        string
    RetryCount     int
    Command        []string
    Logger         *zap.Logger
}

var home string = os.Getenv("HOME")
var KUBECONFIG_APP string = fmt.Sprintf("%s/.kube/kubeconfig_app", home)
var KUBECONFIG_WORKERS string = fmt.Sprintf("%s/.kube/kubeconfig_workers", home)

func CreateJob(job *Job) error {
    // DeleteJob(job)

    // Get the latest image
    appImage, err := getAppImage(job.ServiceName, job.Logger)
    if err != nil {
        job.Logger.Error("Could not get the latest app image.")
        job.Logger.Error(err.Error())
        return err
    }

    job.Logger.Info(fmt.Sprintf("The latest image: %s", appImage))
    job.Logger.Info(fmt.Sprintf("Command: [%s]", strings.Join(job.Command[:], ",")))

    manifestTemplate := `apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: %s\nspec:\n  template:\n    spec:\n      containers:\n        - name: %s\n          image: %s\n          envFrom:\n            - secretRef:\n                name: %s-secret\n          command: [%s]\n      restartPolicy: Never\n  backoffLimit: 1`
    manifest := exec.Command(`/usr/bin/printf`, 
        fmt.Sprintf(manifestTemplate,
            job.JobName,
            job.ServiceName,
            appImage,
            job.ServiceName,
            strings.Join(job.Command[:], ",")))

    // Create new job
    kubectl := exec.Command("kubectl", "apply", "-f", "-")
    kubectl.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", KUBECONFIG_WORKERS))

    r, w := io.Pipe()
    manifest.Stdout = w
    kubectl.Stdin = r

    var manifestStderr bytes.Buffer
    manifest.Stderr = &manifestStderr

    var kubectlStdout bytes.Buffer
    var kubectlStderr bytes.Buffer
    kubectl.Stdout = &kubectlStdout
    kubectl.Stderr = &kubectlStderr

    manifest.Start()
    kubectl.Start()

    err = manifest.Wait()
    if err != nil {
        job.Logger.Error(kubectlStderr.String())
        job.Logger.Error(err.Error())
        return err
    }
    w.Close()

    err = kubectl.Wait()
    if err != nil {
        job.Logger.Error(kubectlStderr.String())
        job.Logger.Error(err.Error())
        return err
    }
    r.Close()

    job.Logger.Info(kubectlStdout.String())
    return nil
}

func TailLog(job *Job) error {
    podName, err := getNewPod(job)
    if err != nil {
        return err
    }

    if len(podName) < 1 {
        return fmt.Errorf("Could not get PodName, please try increasing RETRY_COUNT!")
    }

    job.Logger.Info(fmt.Sprintf("Pod: %s", podName))

    kubectl := exec.Command("kubectl", "logs", "-f", podName)
    kubectl.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", KUBECONFIG_WORKERS))
    var kubectlStderr bytes.Buffer
    kubectl.Stderr = &kubectlStderr
    kubectl.Stderr = &kubectlStderr
    cmdReader, err := kubectl.StdoutPipe()

    if err != nil {
        job.Logger.Error(kubectlStderr.String())
        job.Logger.Error(err.Error())
        return err
    }

    scanner := bufio.NewScanner(cmdReader)
    go func() {
        for scanner.Scan() {
            job.Logger.Info(fmt.Sprintf("%s", scanner.Text()))
        }
    }()

    kubectl.Start()

    err = kubectl.Wait()
    if err != nil {
        job.Logger.Error(kubectlStderr.String())
        job.Logger.Error(err.Error())
        return err
    }

    return nil
}

func IsComplete(job *Job) (bool, error) {
    command := []string{"kubectl", "get", "job", job.JobName, "-o", "jsonpath='{.status.conditions[?(@.type==\"Complete\")].status}'"}
    out, err := execKubectl(command, job.Logger)
    if err != nil {
        job.Logger.Error(err.Error())
        return false, err
    }

    status := strings.Trim(out, "'")

    if status == "True" {
        return true, nil
    }

    return false, nil
}

func DeleteJob(job *Job) error {
    command := []string{"kubectl", "delete", "job", job.JobName}
    _, err := execKubectl(command, job.Logger)

    if err != nil {
        return err
    }

    podName := "dummy"
    for len(podName) > 0 {
        podName, err = getOldPod(job)
        if err != nil {
            return err
        }
        time.Sleep(1 * time.Second)   
    }

    return nil
}

func getAppImage(serviceName string, logger *zap.Logger) (string, error) {
    kubectl := exec.Command("kubectl", "get", "deployment", serviceName, "-o", fmt.Sprintf("jsonpath='{.spec.template.spec.containers[?(@.name==\"%s\")].image}'", serviceName))
    kubectl.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", KUBECONFIG_APP))

    var kubectlStderr bytes.Buffer
    kubectl.Stderr = &kubectlStderr

    out, err := kubectl.Output()

    if err != nil {
        logger.Error(kubectlStderr.String())
        logger.Error(err.Error())
    }

    return fmt.Sprintf("%s", out), err
}

func execKubectl(command []string, logger *zap.Logger) (string, error) {
    kubectl := exec.Command(command[0], command[1:]...)
    kubectl.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", KUBECONFIG_WORKERS))

    var kubectlStderr bytes.Buffer
    kubectl.Stderr = &kubectlStderr

    out, err := kubectl.Output()

    if err != nil {
        logger.Error(fmt.Sprintf("%s", command))
        logger.Error(kubectlStderr.String())
        logger.Error(err.Error())
    }

    return fmt.Sprintf("%s", out), err
}

func getNewPod(job *Job) (string, error) {
    podName := ""
    retryCount := 0
    for len(podName) < 1 && retryCount < job.RetryCount {
        job.Logger.Info(fmt.Sprintf("Try getting Pod: %d", retryCount))
        command := []string{"kubectl", "get", "pod", "-l", fmt.Sprintf("job-name=%s", job.JobName), "-o", "jsonpath='{.items[?(@.status.phase != \"Pending\")].metadata.name}'"}
        out, err := execKubectl(command, job.Logger)

        if err != nil {
            return "", err
        }

        podName = strings.Trim(out, "'")
        retryCount++
        time.Sleep(1 * time.Second)
    }

    if len(podName) > 0 {
        podName = strings.Split(podName, " ")[0]
    }

    return podName, nil
}

func getOldPod(job *Job) (string, error) {
    command := []string{"kubectl", "get", "pod", "-l", fmt.Sprintf("job-name=%s", job.JobName), "-o", "jsonpath='{.items[?(@.status.phase != \"Pending\")].metadata.name}'"}
    out, err := execKubectl(command, job.Logger)
    if err != nil {
        return "", err
    }

    podName := strings.Trim(out, "'")

    if len(podName) > 0 {
        podName = strings.Split(podName, " ")[0]
    }

    return podName, nil
}