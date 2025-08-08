package rule

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/VictoriaMetrics/VictoriaMetrics/app/vmalert/datasource"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/auth"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/prompbmarshal"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/promrelabel"
)

// newTimeSeries first sorts given labels, then returns new time series.
func newTimeSeries(values []float64, timestamps []int64, labels []prompbmarshal.Label) prompbmarshal.TimeSeries {
	promrelabel.SortLabels(labels)
	ts := prompbmarshal.TimeSeries{
		Labels:  labels,
		Samples: make([]prompbmarshal.Sample, len(values)),
	}
	for i := range values {
		ts.Samples[i] = prompbmarshal.Sample{
			Value:     values[i],
			Timestamp: time.Unix(timestamps[i], 0).UnixNano() / 1e6,
		}
	}
	return ts
}

type curlWriter struct {
	b strings.Builder
}

func (cw *curlWriter) string() string {
	res := "curl " + cw.b.String()
	cw.b.Reset()
	return strings.TrimSpace(res)
}

func (cw *curlWriter) addWithEsc(str string) {
	escStr := `'` + strings.ReplaceAll(str, `'`, `'\''`) + `'`
	cw.add(escStr)
}

func (cw *curlWriter) add(str string) {
	cw.b.WriteString(str)
	cw.b.WriteString(" ")
}

func requestToCurl(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}

	cw := &curlWriter{}

	schema := req.URL.Scheme
	requestURL := req.URL.String()
	if !datasource.ShowDatasourceURL() {
		requestURL = req.URL.Redacted()
	}
	if schema == "" {
		schema = "http"
		if req.TLS != nil {
			schema = "https"
		}
		requestURL = schema + "://" + req.Host + requestURL
	}

	if schema == "https" {
		cw.add("-k")
	}

	cw.add("-X")
	cw.add(req.Method)

	var keys []string
	for k := range req.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		cw.add("-H")
		if !datasource.ShowDatasourceURL() && isSecreteHeader(k) {
			cw.addWithEsc(fmt.Sprintf("%s: <secret>", k))
			continue
		}
		cw.addWithEsc(fmt.Sprintf("%s: %s", k, strings.Join(req.Header[k], " ")))
	}

	cw.addWithEsc(requestURL)
	return cw.string()
}

var secretWords = []string{"auth", "pass", "key", "secret", "token"}

func isSecreteHeader(str string) bool {
	s := strings.ToLower(str)
	for _, secret := range secretWords {
		if strings.Contains(s, secret) {
			return true
		}
	}
	return false
}

func isPartialResponse(res datasource.Result) bool {
	if res.IsPartial != nil && *res.IsPartial {
		return true
	}
	return false
}

func addTenantLabelToTSS(auth *auth.Token, tss []prompbmarshal.TimeSeries) {
	if auth == nil {
		return
	}
	accountIDStr := strconv.FormatUint(uint64(auth.AccountID), 10)
	projectIDStr := strconv.FormatUint(uint64(auth.ProjectID), 10)
	for i := range tss {
		ts := &tss[i]
		hasAccountID, hasProjectID := false, false
		for _, label := range ts.Labels {
			labelName := label.Name
			if labelName == "vm_account_id" {
				hasAccountID = true
				label.Value = accountIDStr
				continue
			}
			if labelName == "vm_project_id" {
				hasProjectID = true
				label.Value = projectIDStr
				continue
			}
		}
		needAdd := 0
		if !hasAccountID {
			needAdd++
		}
		if !hasProjectID {
			needAdd++
		}
		if needAdd > 0 {
			if !hasAccountID {
				ts.Labels = append(ts.Labels, prompbmarshal.Label{
					Name:  "vm_account_id",
					Value: accountIDStr,
				})
			}
			if !hasProjectID {
				ts.Labels = append(ts.Labels, prompbmarshal.Label{
					Name:  "vm_project_id",
					Value: projectIDStr,
				})
			}
		}
	}
}
