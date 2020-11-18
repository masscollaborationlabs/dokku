package ps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sh "github.com/codeskyblue/go-sh"
	"github.com/dokku/dokku/plugins/common"
	"github.com/dokku/dokku/plugins/config"
	dockeroptions "github.com/dokku/dokku/plugins/docker-options"
)

// TriggerAppRestart restarts an app
func TriggerAppRestart(appName string) error {
	return Restart(appName)
}

// TriggerCorePostDeploy removes extracted procfiles
// and sets a property to allow the app to be restored on boot
func TriggerCorePostDeploy(appName string) error {
	if err := removeProcfile(appName); err != nil {
		return err
	}

	entries := map[string]string{
		"DOKKU_APP_RESTORE": "1",
	}

	return common.SuppressOutput(func() error {
		return config.SetMany(appName, entries, false)
	})
}

// TriggerInstall initializes app restart policies
func TriggerInstall() error {
	directory := filepath.Join(common.MustGetEnv("DOKKU_LIB_ROOT"), "data", "ps")
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}

	if err := common.SetPermissions(directory, 0755); err != nil {
		return err
	}

	apps, err := common.DokkuApps()
	if err != nil {
		return nil
	}

	for _, appName := range apps {
		policies, err := getRestartPolicy(appName)
		if err != nil {
			return err
		}

		if len(policies) != 0 {
			continue
		}

		if err := dockeroptions.AddDockerOptionToPhases(appName, []string{"deploy"}, "--restart=on-failure:10"); err != nil {
			common.LogWarn(err.Error())
		}
	}

	return nil
}

// TriggerPostAppClone rebuilds the new app
func TriggerPostAppClone(oldAppName string, newAppName string) error {
	if os.Getenv("SKIP_REBUILD") == "true" {
		return nil
	}

	return Rebuild(newAppName)
}

// TriggerPostAppRename rebuilds the renamed app
func TriggerPostAppRename(oldAppName string, newAppName string) error {
	if os.Getenv("SKIP_REBUILD") == "true" {
		return nil
	}

	return Rebuild(newAppName)
}

// TriggerPostCreate ensures apps have a default restart policy
// and scale value for web
func TriggerPostCreate(appName string) error {
	if err := dockeroptions.AddDockerOptionToPhases(appName, []string{"deploy"}, "--restart=on-failure:10"); err != nil {
		return err
	}

	directory := filepath.Join(common.MustGetEnv("DOKKU_LIB_ROOT"), "data", "ps", appName)
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}

	if err := common.SetPermissions(directory, 0755); err != nil {
		return err
	}

	return updateScalefile(appName, make(map[string]int))
}

// TriggerPostDelete destroys the ps properties for a given app container
func TriggerPostDelete(appName string) error {
	return common.PropertyDestroy("ps", appName)
}

// TriggerPostExtract validates a procfile
func TriggerPostExtract(appName string, tempWorkDir string) error {
	procfile := filepath.Join(tempWorkDir, "Procfile")
	if !common.FileExists(procfile) {
		return nil
	}

	b, err := sh.Command("procfile-util", "check", "-P", procfile).CombinedOutput()
	if err != nil {
		return fmt.Errorf(strings.TrimSpace(string(b[:])))
	}
	return nil
}

// TriggerPostStop sets the restore property to false
func TriggerPostStop(appName string) error {
	entries := map[string]string{
		"DOKKU_APP_RESTORE": "0",
	}

	return common.SuppressOutput(func() error {
		return config.SetMany(appName, entries, false)
	})
}

// TriggerPreDeploy ensures an app has an up to date scale file
func TriggerPreDeploy(appName string, imageTag string) error {
	image := common.GetAppImageRepo(appName)
	removeProcfile(appName)

	procfilePath := getProcfilePath(appName)
	if err := extractProcfile(appName, image, procfilePath); err != nil {
		return err
	}

	if err := extractOrGenerateScalefile(appName, image); err != nil {
		return err
	}

	return nil
}

// TriggerProcfileExtract extracted the procfile
func TriggerProcfileExtract(appName string, image string) error {
	directory := filepath.Join(common.MustGetEnv("DOKKU_LIB_ROOT"), "data", "ps", appName)
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}

	if err := common.SetPermissions(directory, 0755); err != nil {
		return err
	}

	procfilePath := getProcfilePath(appName)

	if common.FileExists(procfilePath) {
		if err := common.PlugnTrigger("procfile-remove", []string{appName, procfilePath}...); err != nil {
			return err
		}
	}

	return extractProcfile(appName, image, procfilePath)
}

// TriggerProcfileGetCommand fetches a command from the procfile
func TriggerProcfileGetCommand(appName string, processType string, port int) error {
	procfilePath := getProcfilePath(appName)
	if !common.FileExists(procfilePath) {
		image := common.GetDeployingAppImageName(appName, "", "")
		err := common.SuppressOutput(func() error {
			return common.PlugnTrigger("procfile-extract", []string{appName, image}...)
		})

		if err != nil {
			return err
		}
	}
	command, err := getProcfileCommand(procfilePath, processType, port)
	if err != nil {
		return err
	}

	if command != "" {
		fmt.Printf("%s\n", command)
	}

	return nil
}

// TriggerProcfileRemove removes the procfile if it exists
func TriggerProcfileRemove(appName string, procfilePath string) error {
	if procfilePath == "" {
		procfilePath = getProcfilePath(appName)
	}

	if !common.FileExists(procfilePath) {
		return nil
	}

	os.Remove(procfilePath)
	return nil
}
