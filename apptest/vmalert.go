package apptest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"
)

// Vmalert holds the state of a vmalert app and provides vmalert-specific
// functions.
type Vmalert struct {
	*app
	*ServesMetrics

	httpListenAddr string
	cli            *Client
}

// StartVmalert starts an instance of vmalert with the given flags. It also
// sets the default flags and populates the app instance state with runtime
// values extracted from the application log (such as httpListenAddr)
func StartVmalert(instance string, flags []string, cli *Client, rulesConfigContent string) (*Vmalert, error) {
	// Create temporary rules file with unique name to avoid conflicts between parallel tests
	rulesFile := fmt.Sprintf("%s/%s-rules.yml", os.TempDir(), instance)
	if err := os.WriteFile(rulesFile, []byte(rulesConfigContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to create rules file: %w", err)
	}

	// Add rules file to flags
	flags = append(flags, "-rule="+rulesFile)

	app, stderrExtracts, err := startApp(instance, "../../bin/vmalert", flags, &appOptions{
		defaultFlags: map[string]string{
			"-httpListenAddr": "127.0.0.1:0",
		},
		extractREs: []*regexp.Regexp{
			httpListenAddrRE,
		},
	})
	if err != nil {
		return nil, err
	}

	return &Vmalert{
		app: app,
		ServesMetrics: &ServesMetrics{
			metricsURL: fmt.Sprintf("http://%s/metrics", stderrExtracts[0]),
			cli:        cli,
		},
		httpListenAddr: stderrExtracts[0],
		cli:            cli,
	}, nil
}

// HTTPAddr returns the address at which the vmalert process is
// listening for incoming HTTP requests.
func (app *Vmalert) HTTPAddr() string {
	return app.httpListenAddr
}

// String returns the string representation of the vmalert app state.
func (app *Vmalert) String() string {
	return fmt.Sprintf("{instance: %q httpListenAddr: %q}", app.instance, app.httpListenAddr)
}

// GetRulesAPI retrieves the rules from the /api/v1/rules endpoint
func (app *Vmalert) GetRulesAPI(t *testing.T) (string, int) {
	t.Helper()
	rulesURL := fmt.Sprintf("http://%s/api/v1/rules", app.httpListenAddr)
	return app.cli.Get(t, rulesURL)
}

// GetAlertsAPI retrieves the alerts from the /api/v1/alerts endpoint
func (app *Vmalert) GetAlertsAPI(t *testing.T) (string, int) {
	t.Helper()
	alertsURL := fmt.Sprintf("http://%s/api/v1/alerts", app.httpListenAddr)
	return app.cli.Get(t, alertsURL)
}

// GetGroupsAPI retrieves the groups from the /api/v1/rules endpoint (which includes groups)
func (app *Vmalert) GetGroupsAPI(t *testing.T) (string, int) {
	t.Helper()
	// VMAlert doesn't have a separate /api/v1/groups endpoint, use /api/v1/rules instead
	rulesURL := fmt.Sprintf("http://%s/api/v1/rules", app.httpListenAddr)
	return app.cli.Get(t, rulesURL)
}

// ReloadConfig sends a reload signal to vmalert to reload configuration
func (app *Vmalert) ReloadConfig(t *testing.T) {
	t.Helper()
	reloadURL := fmt.Sprintf("http://%s/-/reload", app.httpListenAddr)
	_, statusCode := app.cli.Post(t, reloadURL, "", nil)
	if statusCode != http.StatusOK {
		t.Fatalf("unexpected status code for config reload: got %d, want %d", statusCode, http.StatusOK)
	}
}

// WaitForRulesLoad waits for vmalert to load and process the rules
func (app *Vmalert) WaitForRulesLoad(t *testing.T, expectedRulesCount int) {
	t.Helper()

	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for rules to load")
		case <-ticker.C:
			rulesResponse, statusCode := app.GetRulesAPI(t)
			if statusCode == http.StatusOK {
				// Parse the response to count rules
				var response struct {
					Data struct {
						Groups []struct {
							Rules []interface{} `json:"rules"`
						} `json:"groups"`
					} `json:"data"`
				}
				if err := json.Unmarshal([]byte(rulesResponse), &response); err == nil {
					totalRules := 0
					for _, group := range response.Data.Groups {
						totalRules += len(group.Rules)
					}
					if totalRules >= expectedRulesCount {
						return
					}
				}
			}
		}
	}
}

// GetConfigReloadSuccessful returns whether the last config reload was successful
func (app *Vmalert) GetConfigReloadSuccessful(t *testing.T) int {
	t.Helper()
	return app.GetIntMetric(t, "vmalert_config_last_reload_successful")
}

// GetConfigReloadTotal returns the total number of config reloads
func (app *Vmalert) GetConfigReloadTotal(t *testing.T) int {
	t.Helper()
	return app.GetIntMetric(t, "vmalert_config_last_reload_total")
}

// GetIterationTotal returns the total number of rule evaluations
func (app *Vmalert) GetIterationTotal(t *testing.T) int {
	t.Helper()
	return app.GetIntMetric(t, "vmalert_iteration_total")
}
