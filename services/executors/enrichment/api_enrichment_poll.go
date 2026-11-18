package enrichment

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	httpservice "ezhealthkonnect/services/http"
)

// pollUntil repeats the step's request until the response's JSON field has
// one of the wanted values, for a call that starts work and then reports its
// progress (a load job's "state"). It returns the response that matched. The
// first response is already in hand; it is checked before any wait.
//
// It fails, with the last value seen, when the timeout passes or ctx ends,
// and fails at once on any request error (the step's own retry settings
// apply to each request, not to the wait).
func pollUntil(ctx context.Context, p *models.APIPollConfig, run func() (*httpservice.Response, error),
	first *httpservice.Response) (*httpservice.Response, error) {

	interval := time.Duration(p.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	timeout := time.Duration(p.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	deadline := time.Now().Add(timeout)

	resp := first
	for {
		last := pollValue(resp, p.UntilField)
		if p.UntilField != "" {
			for _, want := range p.UntilValues {
				if strings.EqualFold(last, want) {
					return resp, nil
				}
			}
		}
		for _, status := range p.UntilStatus {
			if resp.StatusCode == status {
				return resp, nil
			}
		}
		if p.UntilField == "" {
			last = fmt.Sprintf("status %d", resp.StatusCode)
		}
		if time.Now().Add(interval).After(deadline) {
			return nil, fmt.Errorf("polling timed out after %s: %s is still %q", timeout, pollTarget(p), last)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("polling stopped: %w (%s was %q)", ctx.Err(), pollTarget(p), last)
		case <-time.After(interval):
		}
		var err error
		if resp, err = run(); err != nil {
			return nil, err
		}
	}
}

// pollValue reads the watched field of a JSON object response as text; a
// response without it reads as "".
func pollValue(resp *httpservice.Response, field string) string {
	obj, ok := resp.JSON.(map[string]interface{})
	if !ok {
		return ""
	}
	v := executors.GetNestedValue(obj, field)
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// pollTarget names what is being waited on, for messages.
func pollTarget(p *models.APIPollConfig) string {
	if p.UntilField != "" {
		return p.UntilField
	}
	return "the response status"
}
