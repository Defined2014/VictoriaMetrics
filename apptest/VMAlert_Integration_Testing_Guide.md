# VMAlert Integration Testing Guide

## Overview

VMAlert is VictoriaMetrics' alerting and recording rule evaluation engine. This document explains how to run and develop integration tests for VMAlert.

## Quick Start

### Environment Setup

```bash
# Build vmalert binary
make APP_NAME=vmalert app-local-pure

# Verify binary exists
ls -la bin/vmalert*
```

### Running Tests

```bash
# Run all VMAlert tests
go test ./apptest/tests -run="TestCluster.*Vmalert.*" -v

# Run specific test
go test ./apptest/tests -run="TestClusterVmalertBasicFunctionality" -v
```

## Testing Framework

### Core Methods

- `StartVmalert()`: Start VMAlert instance
- `ReloadConfig()`: Reload configuration
- `WaitForRulesLoad()`: Wait for rules to load
- `GetRulesAPI()`: Get rules list
- `GetAlertsAPI()`: Get active alerts
- `GetGroupsAPI()`: Get rule groups information

### Rule Configuration Example

```yaml
groups:
  - name: test_alerts
    interval: 10s
    rules:
      - alert: InstanceDown
        expr: up == 0
        for: 0s
        labels:
          severity: critical
        annotations:
          summary: "Instance {{ $labels.instance }} is down"

  - name: test_recording
    interval: 10s
    rules:
      - record: job:up:avg
        expr: avg by (job) (up)
```

## Troubleshooting

### Common Issues

1. **Missing binary**: `make APP_NAME=vmalert app-local-pure`
2. **Port conflicts**: `pkill -f vmalert`
3. **Rule loading timeout**: Check rule file syntax, increase timeout

### Debugging Tips

```go
// Add debug information in tests
t.Logf("VMAlert HTTP address: %s", vmalert.HTTPAddr())
t.Logf("Rules loaded: %d", vmalert.GetRulesLoadedCount(t))
```

## Extension Development

### Adding New Methods

```go
// Add to apptest/vmalert.go
func (app *Vmalert) GetHealthAPI(t *testing.T) (string, int) {
    t.Helper()
    healthURL := fmt.Sprintf("http://%s/-/healthy", app.httpListenAddr)
    return app.cli.Get(t, healthURL)
}
```

### Testing Best Practices

- Use independent VMAlert instances for each test
- Use retry mechanisms to verify asynchronous operations
- Ensure resource cleanup (`defer tc.Stop()`)

This testing framework ensures VMAlert's stability and reliability across various scenarios.
