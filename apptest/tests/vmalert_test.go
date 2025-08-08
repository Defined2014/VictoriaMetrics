package tests

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/VictoriaMetrics/VictoriaMetrics/apptest"
)

// TestClusterVmalertBasicFunctionality tests basic vmalert functionality with a cluster setup
func TestClusterVmalertBasicFunctionality(t *testing.T) {
	tc := apptest.NewTestCase(t)
	defer tc.Stop()

	// Start a basic cluster
	cluster := tc.MustStartCluster(&apptest.ClusterOptions{
		Vmstorage1Instance: "vmalert-vmstorage1",
		Vmstorage1Flags: []string{
			"-storageDataPath=" + filepath.Join(tc.Dir(), "vmstorage1"),
		},
		Vmstorage2Instance: "vmalert-vmstorage2",
		Vmstorage2Flags: []string{
			"-storageDataPath=" + filepath.Join(tc.Dir(), "vmstorage2"),
		},
		VminsertInstance: "vmalert-vminsert",
		VmselectInstance: "vmalert-vmselect",
	})

	// Insert some test data for alerting rules to evaluate
	testData := []string{
		"up{job=\"test\",instance=\"localhost:8080\"} 1",
		"up{job=\"test\",instance=\"localhost:8081\"} 0",
		"cpu_usage{job=\"test\",instance=\"localhost:8080\"} 85",
		"cpu_usage{job=\"test\",instance=\"localhost:8081\"} 45",
	}

	cluster.PrometheusAPIV1ImportPrometheus(t, testData, apptest.QueryOpts{})
	cluster.ForceFlush(t)

	// Define alerting and recording rules
	rulesConfig := `
groups:
  - name: test_alerts
    interval: 5s
    rules:
      - alert: InstanceDown
        expr: up == 0
        for: 0s
        labels:
          severity: critical
        annotations:
          summary: "Instance {{ $labels.instance }} is down"
          description: "Instance {{ $labels.instance }} has been down for more than 0 seconds."

      - alert: HighCPUUsage
        expr: cpu_usage > 80
        for: 0s
        labels:
          severity: warning
        annotations:
          summary: "High CPU usage on {{ $labels.instance }}"
          description: "CPU usage is {{ $value }}% on instance {{ $labels.instance }}"

  - name: test_recording
    interval: 5s
    rules:
      - record: job:up:avg
        expr: avg by (job) (up)

      - record: job:cpu_usage:avg
        expr: avg by (job) (cpu_usage)
`

	// Start vmalert with the cluster as datasource and remote write target
	vmalertFlags := []string{
		"-datasource.url=http://" + cluster.Vmselect.HTTPAddr() + "/select/0:0/prometheus",
		"-remoteWrite.url=http://" + cluster.Vminsert.HTTPAddr() + "/insert/0:0/prometheus",
		"-evaluationInterval=5s",
		"-remoteWrite.flushInterval=1s", // Flush more frequently for testing
		"-external.url=http://localhost:8880",
		"-notifier.blackhole", // Use blackhole notifier for testing
	}

	vmalert := tc.MustStartVmalert("vmalert-basic", vmalertFlags, rulesConfig)

	// Wait for vmalert to load rules
	vmalert.WaitForRulesLoad(t, 4) // 2 alerting rules + 2 recording rules

	// Test 1: Verify config reload was successful
	tc.Assert(&apptest.AssertOptions{
		Msg: "vmalert config should be loaded successfully",
		Got: func() any {
			return vmalert.GetConfigReloadSuccessful(t)
		},
		Want: 1,
	})

	// Test 2: Verify rules API returns expected number of rules
	tc.Assert(&apptest.AssertOptions{
		Msg: "vmalert should have 4 rules loaded",
		Got: func() any {
			rulesResponse, statusCode := vmalert.GetRulesAPI(t)
			if statusCode != http.StatusOK {
				return -1
			}
			var response struct {
				Data struct {
					Groups []struct {
						Rules []interface{} `json:"rules"`
					} `json:"groups"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(rulesResponse), &response); err != nil {
				return -1
			}
			totalRules := 0
			for _, group := range response.Data.Groups {
				totalRules += len(group.Rules)
			}
			return totalRules
		},
		Want: 4,
	})

	// Test 3: Check rules API endpoint
	tc.Assert(&apptest.AssertOptions{
		Msg: "rules API should return 200",
		Got: func() any {
			_, statusCode := vmalert.GetRulesAPI(t)
			return statusCode
		},
		Want: http.StatusOK,
	})

	// Test 4: Check groups API endpoint
	tc.Assert(&apptest.AssertOptions{
		Msg: "groups API should return 200",
		Got: func() any {
			_, statusCode := vmalert.GetGroupsAPI(t)
			return statusCode
		},
		Want: http.StatusOK,
	})

	// TODO
	// Test 5: Verify alerts API and check for expected alerts
	// Wait a bit for rules to be evaluated and recording rules to be written

	tc.Assert(&apptest.AssertOptions{
		Msg: "alerts API should return 200",
		Got: func() any {
			_, statusCode := vmalert.GetAlertsAPI(t)
			return statusCode
		},
		Want: http.StatusOK,
	})

	// Test 6: Verify that recording rules are written to the cluster
	tc.Assert(&apptest.AssertOptions{
		Msg: "recording rules should be written to cluster",
		Got: func() any {
			// Force flush to ensure data is written
			cluster.ForceFlush(t)
			response := cluster.PrometheusAPIV1Series(t, "job:up:avg", apptest.QueryOpts{})
			return len(response.Data) > 0
		},
		Want:    true,
		Retries: 30,
		Period:  2 * time.Second,
	})

	// Test 7: Verify specific recording rule values
	tc.Assert(&apptest.AssertOptions{
		Msg: "job:up:avg should have correct value",
		Got: func() any {
			// Force flush to ensure data is written
			cluster.ForceFlush(t)
			response := cluster.PrometheusAPIV1Query(t, "job:up:avg", apptest.QueryOpts{})
			if len(response.Data.Result) > 0 {
				return response.Data.Result[0].Sample.Value
			}
			return -1.0
		},
		Want:    0.5, // avg of [1, 0] = 0.5
		Retries: 30,
		Period:  2 * time.Second,
	})
}

// TestClusterVmalertConfigReload tests configuration reload functionality
func TestClusterVmalertConfigReload(t *testing.T) {
	tc := apptest.NewTestCase(t)
	defer tc.Stop()

	cluster := tc.MustStartDefaultCluster()

	// Initial rules configuration with one rule
	initialRulesConfig := `
groups:
  - name: initial_group
    interval: 10s
    rules:
      - record: test:initial
        expr: test_metric * 2
`

	vmalertFlags := []string{
		"-datasource.url=http://" + cluster.Vmselect.HTTPAddr() + "/select/0/prometheus",
		"-remoteWrite.url=http://" + cluster.Vminsert.HTTPAddr() + "/insert/0/prometheus/api/v1/write",
		"-evaluationInterval=5s",
		"-remoteWrite.flushInterval=1s", // Flush more frequently for testing
		"-notifier.blackhole",           // Use blackhole notifier for testing
	}

	vmalert := tc.MustStartVmalert("vmalert-reload", vmalertFlags, initialRulesConfig)
	vmalert.WaitForRulesLoad(t, 1)

	// Verify initial rule count
	tc.Assert(&apptest.AssertOptions{
		Msg: "should initially load 1 rule",
		Got: func() any {
			rulesResponse, statusCode := vmalert.GetRulesAPI(t)
			if statusCode != http.StatusOK {
				return -1
			}
			var response struct {
				Data struct {
					Groups []struct {
						Rules []interface{} `json:"rules"`
					} `json:"groups"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(rulesResponse), &response); err != nil {
				return -1
			}
			// Debug: fmt.Print(response.Data.Groups)
			totalRules := 0
			for _, group := range response.Data.Groups {
				totalRules += len(group.Rules)
			}
			return totalRules
		},
		Want: 1,
	})

	vmalert.ReloadConfig(t)

	// Verify the reload didn't break anything
	tc.Assert(&apptest.AssertOptions{
		Msg: "rules should still be loaded after reload",
		Got: func() any {
			rulesResponse, statusCode := vmalert.GetRulesAPI(t)
			if statusCode != http.StatusOK {
				return -1
			}
			var response struct {
				Data struct {
					Groups []struct {
						Rules []interface{} `json:"rules"`
					} `json:"groups"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(rulesResponse), &response); err != nil {
				return -1
			}
			totalRules := 0
			for _, group := range response.Data.Groups {
				totalRules += len(group.Rules)
			}
			return totalRules
		},
		Want: 1,
	})
}

func TestClusterVmalertMultitenants(t *testing.T) {
	tc := apptest.NewTestCase(t)
	defer tc.Stop()

	// Start a basic cluster
	cluster := tc.MustStartCluster(&apptest.ClusterOptions{
		Vmstorage1Instance: "vmalert-vmstorage",
		Vmstorage1Flags: []string{
			"-storageDataPath=" + filepath.Join(tc.Dir(), "vmstorage"),
		},
		VminsertInstance: "vmalert-vminsert",
		VmselectInstance: "vmalert-vmselect",
	})

	// Insert some test data for alerting rules to evaluate
	testData := []string{
		"up{job=\"test\",instance=\"localhost:8080\"} 1",
		"up{job=\"test\",instance=\"localhost:8081\"} 0",
		"cpu_usage{job=\"test\",instance=\"localhost:8080\"} 85",
		"cpu_usage{job=\"test\",instance=\"localhost:8081\"} 45",
	}

	cluster.PrometheusAPIV1ImportPrometheus(t, testData, apptest.QueryOpts{Tenant: "1:1"})
	cluster.PrometheusAPIV1ImportPrometheus(t, testData, apptest.QueryOpts{Tenant: "1:2"})
	cluster.ForceFlush(t)

	// Define alerting and recording rules
	rulesConfig := `
groups:
  - name: test_alerts_A
    interval: 1s
    tenant: "1:1"
    rules:
      - alert: InstanceDown
        expr: up == 0
        for: 0s
        labels:
          severity: critical
        annotations:
          summary: "Instance {{ $labels.instance }} is down"
          description: "Instance {{ $labels.instance }} has been down for more than 0 seconds."

  - name: test_alterts_B
    interval: 1s
    tenant: "1:2"
    rules:
      - alert: HighCPUUsage
        expr: cpu_usage > 80
        for: 0s
        labels:
          severity: warning
        annotations:
          summary: "High CPU usage on {{ $labels.instance }}"
          description: "CPU usage is {{ $value }}% on instance {{ $labels.instance }}"

  - name: test_recording_A
    interval: 1s
    tenant: "1:1"
    rules:
      - record: job:up:avg
        expr: avg by (job) (up)

  - name: test_recording_B
    interval: 1s
    tenant: "1:2"
    rules:
      - record: job:cpu_usage:avg
        expr: avg by (job) (cpu_usage)
`

	// Start vmalert with the cluster as datasource and remote write target
	vmalertFlags := []string{
		"-datasource.url=http://" + cluster.Vmselect.HTTPAddr() + "/select/multitenant/prometheus",
		"-remoteWrite.url=http://" + cluster.Vminsert.HTTPAddr() + "/insert/multitenant/prometheus",
		"-evaluationInterval=5s",
		"-remoteWrite.flushInterval=1s", // Flush more frequently for testing
		"-external.url=http://localhost:8880",
		"-notifier.blackhole", // Use blackhole notifier for testing
	}

	vmalert := tc.MustStartVmalert("vmalert-multi-tenant", vmalertFlags, rulesConfig)

	// Wait for vmalert to load rules
	vmalert.WaitForRulesLoad(t, 4) // 2 alerting rules + 2 recording rules

	// Verify that recording rules are written to the cluster
	tc.Assert(&apptest.AssertOptions{
		Msg: "recording rules `job:cpu_usage:avg` should be written to tenant `1:2`",
		Got: func() any {
			cluster.ForceFlush(t)
			// Doesn't has `job:cpu_usage:avg` recording rules on `1:1` tenant.
			response := cluster.PrometheusAPIV1Query(t, "job:cpu_usage:avg", apptest.QueryOpts{Tenant: "1:1"})
			if len(response.Data.Result) != 0 {
				return 0
			}
			// Has `job:cpu_usage:avg` recording rules on `1:2` tenant.
			response = cluster.PrometheusAPIV1Query(t, "job:cpu_usage:avg", apptest.QueryOpts{Tenant: "1:2"})
			if len(response.Data.Result) > 0 {
				return response.Data.Result[0].Sample.Value
			}
			return 0
		},
		Want:    65.0,
		Retries: 30,
		Period:  2 * time.Second,
	})

	tc.Assert(&apptest.AssertOptions{
		Msg: "recording rules `job:up:avg` should be written to tenant `1:1`",
		Got: func() any {
			cluster.ForceFlush(t)
			// Doesn't has `job:up:avg` recording rules on `1:2` tenant.
			response := cluster.PrometheusAPIV1Query(t, "job:up:avg", apptest.QueryOpts{Tenant: "1:2"})
			if len(response.Data.Result) != 0 {
				return 0
			}
			// Has `job:up:avg` recording rules on `1:1` tenant.
			response = cluster.PrometheusAPIV1Query(t, "job:up:avg", apptest.QueryOpts{Tenant: "1:1"})
			if len(response.Data.Result) > 0 {
				return response.Data.Result[0].Sample.Value
			}
			return 0
		},
		Want:    0.5,
		Retries: 30,
		Period:  2 * time.Second,
	})

	tc.Assert(&apptest.AssertOptions{
		Msg: "check alters generate successfully",
		Got: func() any {
			cluster.ForceFlush(t)
			if vmalert.GetMetric(t, "vmalert_alerts_fired_total") >= 2 {
				return 1
			}
			return 0
		},
		Want:    1,
		Retries: 30,
		Period:  2 * time.Second,
	})

	tc.Assert(&apptest.AssertOptions{
		Msg: "check alters remote write correclty",
		Got: func() any {
			cluster.ForceFlush(t)
			response := cluster.PrometheusAPIV1Query(t, "count(ALERTS[1h])", apptest.QueryOpts{Tenant: "1:2"})
			if len(response.Data.Result) > 0 {
				return response.Data.Result[0].Sample.Value
			}
			return 0
		},
		Want:    1.0,
		Retries: 30,
		Period:  2 * time.Second,
	})

	tc.Assert(&apptest.AssertOptions{
		Msg: "check alters remote write correclty",
		Got: func() any {
			cluster.ForceFlush(t)
			response := cluster.PrometheusAPIV1Query(t, "count(ALERTS[1h])", apptest.QueryOpts{Tenant: "1:1"})
			if len(response.Data.Result) > 0 {
				return response.Data.Result[0].Sample.Value
			}
			return 0
		},
		Want:    1.0,
		Retries: 30,
		Period:  2 * time.Second,
	})
}
