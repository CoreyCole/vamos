package workspaces

import (
	"net/url"
	"sort"
	"strings"
)

const recoveryBuildComponent BundleComponent = "build"

func recoveryDisplayName(model RecoveryModel) string {
	if strings.TrimSpace(model.Workspace.DisplayName) != "" {
		return model.Workspace.DisplayName
	}
	if strings.TrimSpace(model.Workspace.Slug) != "" {
		return model.Workspace.Slug
	}
	return "Unknown workspace"
}

func recoveryManagerWorkspacesURL(managerURL string) string {
	managerURL = strings.TrimRight(strings.TrimSpace(managerURL), "/")
	if managerURL == "" {
		return "/workspaces"
	}
	return managerURL + "/workspaces"
}

func recoverySwitchURL(model RecoveryModel) string {
	if model.Workspace.Status != StatusRunning ||
		strings.TrimSpace(model.Workspace.URL) == "" {
		return ""
	}
	managerURL := strings.TrimRight(strings.TrimSpace(model.ManagerURL), "/")
	if managerURL == "" {
		managerURL = "/"
	}
	return strings.TrimRight(
		managerURL,
		"/",
	) + "/workspaces/switch/" + model.Workspace.Slug + "?redirect=" + url.QueryEscape(
		model.ReturnTo,
	)
}

func recoveryCanStart(status Status) bool {
	return status == StatusStopped || status == StatusFailed ||
		status == StatusCrashed || status == ""
}

func recoveryCanRetry(status Status) bool {
	return status == StatusFailed || status == StatusCrashed || status == StatusInvalid
}

func recoveryCanStop(status Status) bool {
	return status == StatusStarting || status == StatusRunning || status == StatusStopping
}

func recoverySortedLogComponents(model RecoveryModel) []BundleComponent {
	seen := map[BundleComponent]struct{}{}
	for component, path := range model.Status.Logs {
		if strings.TrimSpace(path) != "" {
			seen[component] = struct{}{}
		}
	}
	if strings.TrimSpace(model.Status.Build.LogPath) != "" ||
		strings.TrimSpace(model.Status.Build.Error) != "" {
		seen[recoveryBuildComponent] = struct{}{}
	}
	for component := range model.LogTails {
		seen[component] = struct{}{}
	}
	components := make([]BundleComponent, 0, len(seen))
	for component := range seen {
		components = append(components, component)
	}
	order := map[BundleComponent]int{
		ComponentWeb:           0,
		ComponentTemporal:      1,
		ComponentTSWorker:      2,
		recoveryBuildComponent: 3,
		ComponentTemporalUI:    4,
	}
	sort.Slice(components, func(i, j int) bool {
		left, lok := order[components[i]]
		right, rok := order[components[j]]
		if lok && rok {
			return left < right
		}
		if lok != rok {
			return lok
		}
		return components[i] < components[j]
	})
	return components
}

func recoveryLogPath(model RecoveryModel, component BundleComponent) string {
	if component == recoveryBuildComponent {
		return model.Status.Build.LogPath
	}
	return model.Status.Logs[component]
}
