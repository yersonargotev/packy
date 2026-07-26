package opencodesmoke

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// observeACPSelections proves the exact host advertises every skill as a native
// slash command and expands each preflighted selection before a local-only,
// deterministic OpenAI-compatible responder accepts it.
func observeACPSelections(ctx context.Context, cfg Config, home, xdg, data, cache, state, work string, skills []expectedSkill, modes []RuntimeModeEvidence) error {
	responder, err := newLocalResponder()
	if err != nil {
		return err
	}
	defer responder.Close()
	config := fmt.Sprintf(`{"model":"packy/fake","permission":{"skill":"allow"},"provider":{"packy":{"npm":"@ai-sdk/openai-compatible","name":"Packy local smoke","options":{"baseURL":%q,"apiKey":"local-smoke"},"models":{"fake":{"name":"Fake"}}}}}`, responder.URL()+"/v1")
	if err := os.WriteFile(filepath.Join(work, "opencode.json"), []byte(config), 0600); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, cfg.OpenCode, "acp", "--cwd", work, "--pure")
	cmd.Dir = work
	cmd.Env = hostEnv(cfg.SearchPath, home, xdg, data, cache, state)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		_ = stdin.Close()
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()
	enc := json.NewEncoder(stdin)
	scan := bufio.NewScanner(stdout)
	scan.Buffer(make([]byte, 1024), 8<<20)
	id := 0
	rpc := func(method string, params any) (int, error) {
		id++
		return id, enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	}
	initID, err := rpc("initialize", map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}})
	if err != nil {
		return err
	}
	if _, err = waitRPC(scan, initID, nil); err != nil {
		return fmt.Errorf("ACP initialize: %w: %s", err, stderr.String())
	}
	newID, err := rpc("session/new", map[string]any{"cwd": work, "mcpServers": []any{}})
	if err != nil {
		return err
	}
	advertised := map[string]bool{}
	result, err := waitRPC(scan, newID, func(msg map[string]any) { collectAvailableCommands(msg, advertised) })
	if err != nil {
		return fmt.Errorf("ACP session/new: %w: %s", err, stderr.String())
	}
	sessionID := nestedString(result, "result", "sessionId")
	if sessionID == "" {
		return errors.New("ACP session/new returned no sessionId")
	}
	// Updates may precede or follow session/new. Read until all skills are advertised.
	deadline := time.Now().Add(10 * time.Second)
	for !allAdvertised(advertised, skills) && time.Now().Before(deadline) {
		if !scan.Scan() {
			break
		}
		var msg map[string]any
		if json.Unmarshal(scan.Bytes(), &msg) == nil {
			collectAvailableCommands(msg, advertised)
		}
	}
	if !allAdvertised(advertised, skills) {
		return fmt.Errorf("ACP did not advertise all nine skills: %v", sortedKeys(advertised))
	}
	byName := map[string]expectedSkill{}
	for _, s := range skills {
		byName[s.name] = s
	}
	for i := range modes {
		parts := strings.SplitN(modes[i].Invocation, " ", 2)
		skill := byName[parts[0]]
		responder.Prepare(skill.name, modes[i].ModeID)
		prompt := fmt.Sprintf("Load the native skill %s for invocation mode %s.", skill.name, modes[i].ModeID)
		promptID, err := rpc("session/prompt", map[string]any{"sessionId": sessionID, "prompt": []any{map[string]any{"type": "text", "text": prompt}}})
		if err != nil {
			return err
		}
		if _, err = waitRPC(scan, promptID, func(msg map[string]any) { collectAvailableCommands(msg, advertised) }); err != nil {
			return fmt.Errorf("ACP selection %s: %w: %s", modes[i].Invocation, err, stderr.String())
		}
		modeObserved, contentObserved, err := responder.WaitSelection(ctx, modes[i].ModeID, skill.content)
		if err != nil {
			return fmt.Errorf("ACP selection %s: %w", modes[i].Invocation, err)
		}
		if !modeObserved || !contentObserved {
			return fmt.Errorf("ACP did not expand exact mode and loaded content for %s: mode=%t content=%t", modes[i].Invocation, modeObserved, contentObserved)
		}
		modes[i].SelectionObserved = true
		modes[i].InvocationAvailable = true
	}
	return nil
}

func requestContains(body, want string) bool {
	var wire any
	if json.Unmarshal([]byte(body), &wire) != nil {
		return false
	}
	found := false
	var visit func(any)
	visit = func(v any) {
		switch x := v.(type) {
		case string:
			found = found || strings.Contains(x, want)
		case map[string]any:
			for _, child := range x {
				visit(child)
			}
		case []any:
			for _, child := range x {
				visit(child)
			}
		}
	}
	visit(wire)
	return found
}

func waitRPC(scan *bufio.Scanner, id int, observe func(map[string]any)) (map[string]any, error) {
	for scan.Scan() {
		var msg map[string]any
		if err := json.Unmarshal(scan.Bytes(), &msg); err != nil {
			return nil, err
		}
		if observe != nil {
			observe(msg)
		}
		if number, ok := msg["id"].(float64); ok && int(number) == id {
			if msg["error"] != nil {
				return nil, fmt.Errorf("RPC error: %v", msg["error"])
			}
			return msg, nil
		}
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	return nil, ioEOF{}
}

type ioEOF struct{}

func (ioEOF) Error() string { return "ACP stdout closed" }
func nestedString(m map[string]any, keys ...string) string {
	var v any = m
	for _, k := range keys {
		x, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		v = x[k]
	}
	s, _ := v.(string)
	return s
}
func collectAvailableCommands(msg map[string]any, out map[string]bool) {
	var visit func(any)
	visit = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if kind, _ := x["sessionUpdate"].(string); kind == "available_commands_update" {
				for _, key := range []string{"availableCommands", "commands"} {
					if list, ok := x[key].([]any); ok {
						for _, item := range list {
							if command, ok := item.(map[string]any); ok {
								if name, ok := command["name"].(string); ok {
									out[strings.TrimPrefix(name, "/")] = true
								}
							}
						}
					}
				}
			}
			for _, child := range x {
				visit(child)
			}
		case []any:
			for _, child := range x {
				visit(child)
			}
		}
	}
	visit(msg)
}
func allAdvertised(got map[string]bool, want []expectedSkill) bool {
	for _, s := range want {
		if !got[s.name] {
			return false
		}
	}
	return true
}
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type localResponder struct {
	server   *http.Server
	listener net.Listener
	bodies   chan string
	mu       sync.Mutex
	skill    string
	mode     string
	issued   bool
}

func newLocalResponder() (*localResponder, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	r := &localResponder{listener: listener, bodies: make(chan string, 16)}
	r.server = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var b bytes.Buffer
		_, _ = b.ReadFrom(req.Body)
		body := b.Bytes()
		if req.Method == http.MethodPost && json.Valid(body) &&
			(bytes.Contains(body, []byte(`"messages"`)) || bytes.Contains(body, []byte(`"input"`))) {
			select {
			case r.bodies <- string(body):
			default:
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, r.response(string(body)))
	})}
	go r.server.Serve(listener)
	return r, nil
}
func (r *localResponder) URL() string { return "http://" + r.listener.Addr().String() }
func (r *localResponder) Prepare(skill, mode string) {
	r.mu.Lock()
	r.skill, r.mode, r.issued = skill, mode, false
	r.mu.Unlock()
	for {
		select {
		case <-r.bodies:
		default:
			return
		}
	}
}
func (r *localResponder) response(body string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.issued && r.skill != "" && strings.Contains(body, r.mode) &&
		!strings.Contains(body, "You are a title generator") {
		r.issued = true
		args, _ := json.Marshal(map[string]string{"name": r.skill})
		return fmt.Sprintf("data: {\"id\":\"local\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"fake\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call_local\",\"type\":\"function\",\"function\":{\"name\":\"skill\",\"arguments\":%q}}]},\"finish_reason\":null}]}\n\ndata: {\"id\":\"local\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"fake\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", string(args))
	}
	return "data: {\"id\":\"local\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"fake\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"local\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"fake\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
}
func (r *localResponder) WaitSelection(ctx context.Context, mode, content string) (bool, bool, error) {
	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()
	modeObserved, contentObserved := false, false
	for {
		select {
		case body := <-r.bodies:
			modeObserved = modeObserved || requestContains(body, mode)
			contentObserved = contentObserved || requestContains(body, content)
			if modeObserved && contentObserved {
				return true, true, nil
			}
		case <-ctx.Done():
			return modeObserved, contentObserved, ctx.Err()
		case <-timeout.C:
			return modeObserved, contentObserved, errors.New("local responder did not receive the expanded native-skill selection")
		}
	}
}
func (r *localResponder) Close() { _ = r.server.Close() }
