package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/packclassification"
	"github.com/yersonargotev/packy/internal/packsync"
	"github.com/yersonargotev/packy/internal/packsyncworkflow"
)

func TestGitHubModelRequestsExactStrictClassificationSchema(t *testing.T) {
	var payload map[string]any
	model := modelForTest(func(request *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return modelResponse(http.StatusOK, validMajorCompletion()), nil
	})
	if _, err := model.attempt(context.Background(), classificationRequestForTest()); err != nil {
		t.Fatal(err)
	}

	var expected map[string]any
	if err := json.Unmarshal([]byte(`{
		"type":"json_schema",
		"json_schema":{
			"name":"packy_classification_evidence",
			"strict":true,
			"schema":{
				"type":"object",
				"additionalProperties":false,
				"properties":{
					"pack_id":{"type":"string","enum":["vercel"]},
					"classifier":{
						"type":"object",
						"additionalProperties":false,
						"properties":{
							"type":{"type":"string","enum":["ai"]},
							"id":{"type":"string","enum":["openai/gpt-4.1"]}
						},
						"required":["type","id"]
					},
					"rationale":{"type":"string","minLength":1,"maxLength":500},
					"current_version":{"type":"string","enum":["0.0.0"]},
					"proposed_version":{"type":"string","pattern":"^(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)$"},
					"changed_aspects":{"type":"array","items":{"type":"string","minLength":1},"minItems":1},
					"mechanical_floor":{"type":"string","enum":["major"]},
					"final_level":{"type":"string","enum":["patch","minor","major"]},
					"migration":{"type":"string"},
					"required_actions":{"type":"array","items":{"type":"string","minLength":1}}
				},
				"required":["pack_id","classifier","rationale","current_version","proposed_version","changed_aspects","mechanical_floor","final_level","migration","required_actions"]
			}
		}
	}`), &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(payload["response_format"], expected) {
		t.Fatalf("response_format = %#v", payload["response_format"])
	}
}

func TestGitHubModelAcceptsValidMajorStructuredEvidence(t *testing.T) {
	model := modelForTest(func(*http.Request) (*http.Response, error) {
		return modelResponse(http.StatusOK, validMajorCompletion()), nil
	})
	evidence, err := model.attempt(context.Background(), classificationRequestForTest())
	if err != nil {
		t.Fatal(err)
	}
	if evidence.PackID != "vercel" || evidence.Classifier != (packsync.ClassifierIdentity{Type: packsync.ClassifierAI, ID: defaultModel}) ||
		evidence.FinalLevel != packsync.LevelMajor || evidence.ProposedVersion != "1.0.0" || len(evidence.RequiredActions) != 1 {
		t.Fatalf("evidence = %#v", evidence)
	}
	if len(model.traces) != 1 || model.traces[0].PackID != "vercel" || model.traces[0].Model != defaultModel {
		t.Fatalf("traces = %#v", model.traces)
	}
}

func TestGitHubModelRejectsInvalidStructuredEvidence(t *testing.T) {
	valid := validMajorEvidenceJSON()
	tests := []struct {
		name    string
		content string
	}{
		{name: "malformed", content: `{"pack_id":`},
		{name: "unknown root field", content: strings.TrimSuffix(valid, "}") + `,"unexpected":true}`},
		{name: "unknown classifier field", content: strings.Replace(valid, `"id":"openai/gpt-4.1"`, `"id":"openai/gpt-4.1","unexpected":true`, 1)},
		{name: "null migration", content: strings.Replace(valid, `"migration":"Introduce the Vercel Pack."`, `"migration":null`, 1)},
		{name: "null required actions", content: strings.Replace(valid, `"required_actions":["Review the initial Pack."]`, `"required_actions":null`, 1)},
		{name: "trailing object", content: valid + `{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := modelForTest(func(*http.Request) (*http.Response, error) {
				return modelResponse(http.StatusOK, completionFor(test.content)), nil
			})
			_, err := model.attempt(context.Background(), classificationRequestForTest())
			var failure packsyncworkflow.Failure
			if !errors.As(err, &failure) || failure.Kind != packsyncworkflow.FailureClassification ||
				!strings.Contains(err.Error(), "invalid classification JSON") {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestGitHubModelRetriesOnlyTransientHTTPFailures(t *testing.T) {
	t.Run("unprocessable response is terminal", func(t *testing.T) {
		calls := 0
		model := modelForTest(func(*http.Request) (*http.Response, error) {
			calls++
			return modelResponse(http.StatusUnprocessableEntity, `{}`), nil
		})
		model.retry = packsyncworkflow.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Nanosecond, Sleeper: modelNoWaitSleeper{}}
		_, err := model.Attempt(context.Background(), classificationRequestForTest())
		var failure packsyncworkflow.Failure
		if calls != 1 || !errors.As(err, &failure) || failure.Kind != packsyncworkflow.FailureClassification {
			t.Fatalf("calls=%d error=%#v", calls, err)
		}
	})

	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			model := modelForTest(func(*http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return modelResponse(status, `{}`), nil
				}
				return modelResponse(http.StatusOK, validMajorCompletion()), nil
			})
			model.retry = packsyncworkflow.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Nanosecond, Sleeper: modelNoWaitSleeper{}}
			if _, err := model.Attempt(context.Background(), classificationRequestForTest()); err != nil || calls != 2 {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
}

func classificationRequestForTest() packclassification.Request {
	return packclassification.Request{
		SchemaVersion: 1, RequestID: "plan/vercel", Mode: packclassification.ModeAI,
		PlanID: "plan", BaseSHA: strings.Repeat("a", 40), PackID: "vercel",
		CurrentVersion: "0.0.0", MechanicalFloor: packsync.LevelMajor,
		SemanticEvidenceRequired: true, MechanicalReasons: []string{"initial complete composite Pack generation"},
	}
}

func validMajorEvidenceJSON() string {
	return `{"pack_id":"vercel","classifier":{"type":"ai","id":"openai/gpt-4.1"},"rationale":"Initial complete Pack admission.","current_version":"0.0.0","proposed_version":"1.0.0","changed_aspects":["complete Vercel Pack"],"mechanical_floor":"major","final_level":"major","migration":"Introduce the Vercel Pack.","required_actions":["Review the initial Pack."]}`
}

func validMajorCompletion() string {
	return completionFor(validMajorEvidenceJSON())
}

func completionFor(content string) string {
	data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
	return string(data)
}

func modelForTest(roundTrip func(*http.Request) (*http.Response, error)) *githubModel {
	return &githubModel{token: "token", model: defaultModel, client: &http.Client{Transport: modelRoundTripFunc(roundTrip)}}
}

func modelResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

type modelRoundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip modelRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

type modelNoWaitSleeper struct{}

func (modelNoWaitSleeper) Sleep(context.Context, time.Duration) error { return nil }
