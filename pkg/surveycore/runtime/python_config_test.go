package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	surveyconfig "github.com/SurveyController/SurveyCore/pkg/surveycore/config"
)

func TestRunImportedPythonConfigPreservesAnswersAndUserAgent(t *testing.T) {
	data, err := os.ReadFile("../config/testdata/python-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	request, err := surveyconfig.DeserializeRunRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var submissions []string
	var agents []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/vm/demo.aspx":
			_, _ = w.Write([]byte(`<html><head><title>Python fixture</title></head><body><div id="divQuestion"><fieldset>
<div topic="1" id="div1" type="3"><div class="topichtml">Choice</div><div class="ui-controlgroup"><div><span class="label">A</span></div><div><span class="label">B</span></div></div></div>
</fieldset></div></body></html>`))
		case "/joinnew/processjq.ashx":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			mu.Lock()
			submissions = append(submissions, r.Form.Get("submitdata"))
			agents = append(agents, r.UserAgent())
			mu.Unlock()
			_, _ = w.Write([]byte("10"))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	result, err := New(WithHTTPClient(rewriteWJXHTTPClient(server.URL))).Run(context.Background(), &request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success != 3 || result.Fail != 0 {
		t.Fatalf("result = %#v", result)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(submissions) != 3 {
		t.Fatalf("submissions = %#v", submissions)
	}
	for _, answer := range submissions {
		if answer != "1$2" {
			t.Fatalf("imported [0,1] weights were overwritten: %q", answer)
		}
	}
	for _, agent := range agents {
		if !strings.Contains(agent, "Windows NT 10.0") || !strings.Contains(agent, "Chrome/121") {
			t.Fatalf("unexpected user-agent profile: %q", agent)
		}
	}
	if len(agents) != 3 {
		t.Fatalf("requests = %#v", agents)
	}
	if options := ExecutionOptionsFromConfig(&request); !options.UserAgent.Enabled || options.UserAgent.Ratios["pc"] != 100 || options.Threads != 2 {
		t.Fatalf("execution options = %#v", options)
	}
}
