package diagnostics

import (
	"fmt"
	"log"
	"strings"

	"github.com/Azure/aks-mcp/internal/azcli"
	"github.com/Azure/aks-mcp/internal/components/common"
	"github.com/Azure/aks-mcp/internal/config"
	"github.com/Azure/aks-mcp/internal/tools"
)

// buildClusterResourceID constructs the Azure resource ID for an AKS cluster
func buildClusterResourceID(subscriptionID, resourceGroup, clusterName string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerService/managedClusters/%s",
		subscriptionID, resourceGroup, clusterName)
}

// HandleControlPlaneDiagnosticSettings checks diagnostic settings for AKS cluster
func HandleControlPlaneDiagnosticSettings(params map[string]interface{}, cfg *config.ConfigData) (string, error) {
	// Extract and validate parameters using common helper
	subscriptionID, resourceGroup, clusterName, err := common.ExtractAKSParameters(params)
	if err != nil {
		return "", err
	}

	// Build cluster resource ID using utility function
	clusterResourceID := buildClusterResourceID(subscriptionID, resourceGroup, clusterName)

	// Execute Azure CLI command to get diagnostic settings
	executor := azcli.NewExecutor()
	args := []string{
		"monitor", "diagnostic-settings", "list",
		"--resource", clusterResourceID,
		"--output", "json",
	}

	cmdParams := map[string]interface{}{
		"command": "az " + strings.Join(args, " "),
	}

	result, err := executor.Execute(cmdParams, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to get diagnostic settings for cluster %s in resource group %s: %w", clusterName, resourceGroup, err)
	}

	// Return raw JSON result from Azure CLI
	return result, nil
}

// HandleControlPlaneLogs queries specific control plane logs
func HandleControlPlaneLogs(params map[string]interface{}, cfg *config.ConfigData) (string, error) {
	// Extract and validate AKS parameters using common helper
	subscriptionID, resourceGroup, clusterName, err := common.ExtractAKSParameters(params)
	if err != nil {
		return "", err
	}

	// Extract remaining parameters
	logCategory, _ := params["log_category"].(string)
	startTime, _ := params["start_time"].(string)
	endTime, _ := params["end_time"].(string)
	maxRecords := GetMaxRecords(params)
	logLevel, _ := params["log_level"].(string)

	// Validate parameters
	if err := ValidateControlPlaneLogsParams(params); err != nil {
		return "", err
	}

	// Get workspace GUID from diagnostic settings
	workspaceGUID, err := ExtractWorkspaceGUIDFromDiagnosticSettings(subscriptionID, resourceGroup, clusterName, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to get workspace GUID for cluster %s: %w", clusterName, err)
	}

	// Build cluster resource ID for scoping using utility function
	clusterResourceID := buildClusterResourceID(subscriptionID, resourceGroup, clusterName)

	// Build safe KQL query scoped to this specific AKS cluster
	kqlQuery := BuildSafeKQLQuery(logCategory, logLevel, maxRecords, clusterResourceID)

	// Calculate timespan for the query
	timespan, err := CalculateTimespan(startTime, endTime)
	if err != nil {
		return "", fmt.Errorf("failed to calculate timespan: %w", err)
	}

	// Execute log query with properly quoted KQL
	executor := azcli.NewExecutor()

	// Build command string with proper quoting for the KQL query
	cmd := fmt.Sprintf("az monitor log-analytics query --workspace %s --analytics-query \"%s\" --timespan %s --output json",
		workspaceGUID, kqlQuery, timespan)

	// Log the query command for debugging
	log.Printf("Executing KQL query command: %s", cmd)

	cmdParams := map[string]interface{}{
		"command": cmd,
	}

	result, err := executor.Execute(cmdParams, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to query control plane logs for category %s in cluster %s: %w", logCategory, clusterName, err)
	}

	// Return raw JSON result from Azure CLI
	return result, nil
}

// Resource handler functions for control plane diagnostics tools

// GetControlPlaneDiagnosticSettingsHandler returns handler for diagnostic settings tool
func GetControlPlaneDiagnosticSettingsHandler(cfg *config.ConfigData) tools.ResourceHandler {
	return tools.ResourceHandlerFunc(func(params map[string]interface{}, _ *config.ConfigData) (string, error) {
		return HandleControlPlaneDiagnosticSettings(params, cfg)
	})
}

// GetControlPlaneLogsHandler returns handler for logs querying tool
func GetControlPlaneLogsHandler(cfg *config.ConfigData) tools.ResourceHandler {
	return tools.ResourceHandlerFunc(func(params map[string]interface{}, _ *config.ConfigData) (string, error) {
		return HandleControlPlaneLogs(params, cfg)
	})
}
