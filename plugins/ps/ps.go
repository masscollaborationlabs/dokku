package ps

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dokku/dokku/plugins/common"
)

// RunInSerial is the default value for whether to run a command in parallel or not
// and defaults to -1 (false)
const RunInSerial = -1

// ParallelCommand is a type that declares functions
// the ps plugins can execute in parallel
type ParallelCommand func(string) error

var (
	// DefaultProperties is a map of all valid ps properties with corresponding default property values
	DefaultProperties = map[string]string{
		"restart-policy": "on-failure:10",
	}
)

// RunCommandAgainstAllApps runs a given ParallelCommand against all apps
func RunCommandAgainstAllApps(command ParallelCommand, commandName string, runInSerial bool, parallelCount int) error {
	if runInSerial && parallelCount != RunInSerial {
		common.LogWarn("Ignoring --parallel value and running in serial mode")
	}

	if runInSerial {
		return RunCommandAgainstAllAppsSerially(command, commandName)
	}

	return RunCommandAgainstAllAppsInParallel(command, commandName, parallelCount)
}

// RunCommandAgainstAllAppsInParallel runs a given
// ParallelCommand against all apps in parallel
// TODO: implement me
func RunCommandAgainstAllAppsInParallel(command ParallelCommand, commandName string, parallelCount int) error {
	return nil
}

// RunCommandAgainstAllAppsSerially runs a given
// ParallelCommand against all apps in serial
func RunCommandAgainstAllAppsSerially(command ParallelCommand, commandName string) error {
	apps, err := common.DokkuApps()
	if err != nil {
		common.LogWarn(err.Error())
		return nil
	}

	errorCount := 0
	for _, appName := range apps {
		if err = command(appName); err != nil {
			errorCount++
		}
	}

	if errorCount > 0 {
		return fmt.Errorf("%s command returned %d errors", commandName, errorCount)
	}

	return nil
}

// Rebuild rebuilds app from base image
func Rebuild(appName string) error {
	return common.PlugnTrigger("receive-app", []string{appName}...)
}

// ReportSingleApp is an internal function that displays the ps report for one or more apps
func ReportSingleApp(appName, infoFlag string) error {
	if err := common.VerifyAppName(appName); err != nil {
		return err
	}

	policy, _ := getRestartPolicy(appName)
	if policy == "" {
		policy = DefaultProperties["restart-policy"]
	}

	canScale := "false"
	if canScaleApp(appName) {
		canScale = "true"
	}

	deployed := "false"
	if common.IsDeployed(appName) {
		deployed = "true"
	}

	runningState := getRunningState(appName)

	count, err := getProcessCount(appName)
	if err != nil {
		count = -1
	}

	b, _ := common.PlugnTriggerOutput("config-get", []string{appName, "DOKKU_APP_RESTORE"}...)
	restore := strings.TrimSpace(string(b[:]))
	if restore == "0" {
		restore = "false"
	} else {
		restore = "true"
	}

	infoFlags := map[string]string{
		"--deployed":          deployed,
		"--processes":         strconv.Itoa(count),
		"--ps-can-scale":      canScale,
		"--ps-restart-policy": policy,
		"--restore":           restore,
		"--running":           runningState,
	}

	scheduler := common.GetAppScheduler(appName)
	if scheduler == "docker-local" {
		processStatus := getProcessStatus(appName)
		for process, value := range processStatus {
			infoFlags[fmt.Sprintf("--status-%s", process)] = value
		}
	}

	trimPrefix := false
	uppercaseFirstCharacter := true
	return common.ReportSingleApp("ps", appName, infoFlag, infoFlags, trimPrefix, uppercaseFirstCharacter)
}

// Restart restarts the app
func Restart(appName string) error {
	if !common.IsDeployed(appName) {
		common.LogWarn(fmt.Sprintf("App %s has not been deployed", appName))
		return nil
	}

	return common.PlugnTrigger("release-and-deploy", []string{appName}...)
}

// Start starts the app
func Start(appName string) error {
	imageTag, _ := common.GetRunningImageTag(appName)

	if !common.IsDeployed(appName) {
		common.LogWarn(fmt.Sprintf("App %s has not been deployed", appName))
		return nil
	}

	if err := common.PlugnTrigger("pre-start", []string{appName}...); err != nil {
		return fmt.Errorf("Failure in pre-start hook: %s", err)
	}

	runningState := getRunningState(appName)

	if runningState == "mixed" {
		common.LogWarn("App is running in mixed mode, releasing")
	}

	if runningState != "true" {
		if err := common.PlugnTrigger("release-and-deploy", []string{appName, imageTag}...); err != nil {
			return err
		}
	} else {
		common.LogWarn(fmt.Sprintf("App %s already running", appName))
	}

	if err := common.PlugnTrigger("proxy-build-config", []string{appName}...); err != nil {
		return err
	}

	return nil
}

// Stop stops the app
func Stop(appName string) error {
	if !common.IsDeployed(appName) {
		common.LogWarn(fmt.Sprintf("App %s has not been deployed", appName))
		return nil
	}

	common.LogQuiet(fmt.Sprintf("Stopping %s", appName))
	scheduler := common.GetAppScheduler(appName)

	if err := common.PlugnTrigger("scheduler-stop", []string{scheduler, appName}...); err != nil {
		return err
	}

	if err := common.PlugnTrigger("post-stop", []string{appName}...); err != nil {
		return err
	}

	return nil
}
