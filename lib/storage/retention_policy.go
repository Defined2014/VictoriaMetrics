package storage

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/VictoriaMetrics/VictoriaMetrics/lib/flagutil"
)

type retentionField uint8

const (
	retentionFieldAccount retentionField = iota
	retentionFieldProject
)

type retentionOp uint8

const (
	retentionOpEq retentionOp = iota
	retentionOpNe
)

type retentionMatcher struct {
	field retentionField
	op    retentionOp
	value uint32
}

type retentionRule struct {
	matchers       []retentionMatcher
	retentionMsecs int64
}

type retentionPolicy struct {
	globalRetentionMsecs int64
	rules                []retentionRule
}

func newRetentionPolicy(globalRetention time.Duration, rawRules []string) (*retentionPolicy, error) {
	globalRetentionMsecs := globalRetention.Milliseconds()
	if globalRetentionMsecs <= 0 {
		return nil, fmt.Errorf("global retention must be positive; got %dms", globalRetentionMsecs)
	}
	p := &retentionPolicy{
		globalRetentionMsecs: globalRetentionMsecs,
	}
	for _, rawRule := range rawRules {
		rule, err := parseRetentionRule(rawRule, globalRetentionMsecs)
		if err != nil {
			return nil, err
		}
		p.rules = append(p.rules, rule)
	}
	return p, nil
}

func (p *retentionPolicy) hasRules() bool {
	return p != nil && len(p.rules) > 0
}

func (p *retentionPolicy) retentionMsecsForTenant(accountID, projectID uint32) int64 {
	if p == nil {
		return 0
	}
	retentionMsecs := p.globalRetentionMsecs
	for _, rule := range p.rules {
		if !rule.matches(accountID, projectID) {
			continue
		}
		if rule.retentionMsecs < retentionMsecs {
			retentionMsecs = rule.retentionMsecs
		}
	}
	return retentionMsecs
}

func (r *retentionRule) matches(accountID, projectID uint32) bool {
	for _, m := range r.matchers {
		v := uint32(0)
		switch m.field {
		case retentionFieldAccount:
			v = accountID
		case retentionFieldProject:
			v = projectID
		default:
			return false
		}
		ok := v == m.value
		if m.op == retentionOpNe {
			ok = !ok
		}
		if !ok {
			return false
		}
	}
	return true
}

func parseRetentionRule(rawRule string, globalRetentionMsecs int64) (retentionRule, error) {
	rawRule = strings.TrimSpace(rawRule)
	if rawRule == "" {
		return retentionRule{}, fmt.Errorf("empty -retentionRule")
	}
	n := strings.LastIndexByte(rawRule, ':')
	if n < 0 {
		return retentionRule{}, fmt.Errorf("missing duration in -retentionRule=%q; expecting matcher:duration", rawRule)
	}
	matcherExpr := strings.TrimSpace(rawRule[:n])
	durationExpr := strings.TrimSpace(rawRule[n+1:])
	if matcherExpr == "" {
		return retentionRule{}, fmt.Errorf("missing matcher in -retentionRule=%q", rawRule)
	}
	if durationExpr == "" {
		return retentionRule{}, fmt.Errorf("missing duration in -retentionRule=%q", rawRule)
	}

	d, err := parseRetentionDuration(durationExpr)
	if err != nil {
		return retentionRule{}, fmt.Errorf("cannot parse duration in -retentionRule=%q: %w", rawRule, err)
	}
	retentionMsecs := d.Milliseconds()
	if retentionMsecs > globalRetentionMsecs {
		return retentionRule{}, fmt.Errorf("retention in -retentionRule=%q exceeds -retentionPeriod", rawRule)
	}

	rawMatchers := strings.Split(matcherExpr, ",")
	matchers := make([]retentionMatcher, 0, len(rawMatchers))
	for _, rawMatcher := range rawMatchers {
		m, err := parseRetentionMatcher(rawMatcher)
		if err != nil {
			return retentionRule{}, fmt.Errorf("cannot parse matcher in -retentionRule=%q: %w", rawRule, err)
		}
		matchers = append(matchers, m)
	}
	if len(matchers) == 0 {
		return retentionRule{}, fmt.Errorf("no matchers in -retentionRule=%q", rawRule)
	}

	return retentionRule{
		matchers:       matchers,
		retentionMsecs: retentionMsecs,
	}, nil
}

func parseRetentionMatcher(raw string) (retentionMatcher, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return retentionMatcher{}, fmt.Errorf("empty matcher")
	}
	if strings.Count(raw, "!=") > 1 || strings.Count(raw, "=") > 1 {
		return retentionMatcher{}, fmt.Errorf("invalid matcher %q", raw)
	}
	op := retentionOpEq
	parts := strings.SplitN(raw, "=", 2)
	if strings.Contains(raw, "!=") {
		op = retentionOpNe
		parts = strings.SplitN(raw, "!=", 2)
	}
	if len(parts) != 2 {
		return retentionMatcher{}, fmt.Errorf("invalid matcher %q", raw)
	}
	k := strings.TrimSpace(parts[0])
	v := strings.TrimSpace(parts[1])
	if k == "" || v == "" {
		return retentionMatcher{}, fmt.Errorf("invalid matcher %q", raw)
	}

	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return retentionMatcher{}, fmt.Errorf("cannot parse value %q: %w", v, err)
	}

	field := retentionFieldAccount
	switch k {
	case "account":
		field = retentionFieldAccount
	case "project":
		field = retentionFieldProject
	default:
		return retentionMatcher{}, fmt.Errorf("unsupported matcher field %q; expecting account or project", k)
	}
	return retentionMatcher{
		field: field,
		op:    op,
		value: uint32(n),
	}, nil
}

func parseRetentionDuration(value string) (time.Duration, error) {
	var d flagutil.RetentionDuration
	if err := d.Set(value); err != nil {
		return 0, err
	}
	duration := d.Duration()
	if duration <= 0 {
		return 0, fmt.Errorf("duration must be positive; got %q", value)
	}
	return duration, nil
}
