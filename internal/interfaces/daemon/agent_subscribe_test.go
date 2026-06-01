package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
	"github.com/stretchr/testify/require"
)

func TestAgentManagerAttachAndSubscribe(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(runtime.NewEngine(runtime.NewMemStore()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := mgr.Subscribe(ctx, "session-fake")
	require.NotNil(t, events)

	var wg sync.WaitGroup
	wg.Add(1)
	got := make([]string, 0, 4)
	go func() {
		defer wg.Done()
		for ev := range events {
			got = append(got, ev.Type)
		}
	}()

	// Drive a small deterministic turn through the engine.
	eng := mgr.engine
	_, _ = eng.Submit(ctx, runtime.Task{ID: "tx", SessionID: "session-fake", Origin: "user", Goal: "hi"})
	time.Sleep(40 * time.Millisecond)

	cancel()
	wg.Wait()

	require.Contains(t, got, "task_created", "expected task_created event, got %v", got)
}

func TestAgentManagerSubscribeNilEngine(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	// No AttachEngine — should return a closed channel immediately.
	ch := mgr.Subscribe(context.Background(), "s")
	require.NotNil(t, ch)
	_, ok := <-ch
	require.False(t, ok, "expected closed channel when engine is nil")
}

func TestHTTPAgentSessionStreamForwardsRuntimeEvents(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr.AttachEngine(eng)
	created, err := mgr.CreateSession(createAgentSessionRequest{Operator: "tester", Language: "zh"})
	require.NoError(t, err)
	sessionID := created.Session.SessionID

	api := httptest.NewServer(newRouter(routerServices{agent: mgr}, "test-token"))
	defer api.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api.URL+"/api/v1/agent/sessions/"+sessionID+"/stream", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	go func() {
		time.Sleep(40 * time.Millisecond)
		eng.Publish(context.Background(), runtime.Event{
			Type:   "model_token",
			TaskID: "task-stream",
			Data:   map[string]any{"session_id": sessionID, "text": "hello"},
		})
	}()

	reader := bufio.NewReader(resp.Body)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for runtime model_token SSE event")
		default:
		}
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		if strings.TrimSpace(line) != "event: model_token" {
			continue
		}
		dataLine, err := reader.ReadString('\n')
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(dataLine, "data: "), "unexpected data line %q", dataLine)
		var payload runtime.Event
		require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(dataLine, "data: "))), &payload))
		require.Equal(t, "model_token", payload.Type)
		require.Equal(t, sessionID, payload.Data["session_id"])
		require.Equal(t, "hello", payload.Data["text"])
		return
	}
}

func TestAgentManagerStreamsLLMDeltasToRuntime(t *testing.T) {
	store := runtime.NewMemStore()
	eng := runtime.NewEngine(store)
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)
	created, err := mgr.CreateSession(createAgentSessionRequest{Operator: "tester", Language: "en"})
	require.NoError(t, err)
	sessionID := created.Session.SessionID

	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, true, body["stream"])

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer llmServer.Close()

	resp, err := mgr.SendMessage(context.Background(), sessionID, sendAgentMessageRequest{
		Operator: "tester",
		Language: "en",
		Message:  "Say hello",
		Provider: "openai",
		Model:    "gpt-test",
		APIKey:   "test-key",
		BaseURL:  llmServer.URL + "/v1",
	}, routerServices{})
	require.NoError(t, err)
	require.Len(t, resp.Messages, 2)
	require.Contains(t, resp.Messages[1].Content, "Hello world")

	var tokenDeltas []string
	var llmToolFinished bool
	for _, ev := range store.Events() {
		if ev.Type != "model_token" && ev.Type != "tool_finished" {
			continue
		}
		require.Equal(t, sessionID, ev.Data["session_id"])
		switch ev.Type {
		case "model_token":
			tokenDeltas = append(tokenDeltas, ev.Data["delta"].(string))
			require.Equal(t, "plan", ev.Data["phase"])
		case "tool_finished":
			if ev.Data["tool"] == "LLM chat" {
				llmToolFinished = true
			}
		}
	}
	require.Equal(t, []string{"Hello", " world"}, tokenDeltas)
	require.True(t, llmToolFinished, "expected final LLM chat tool_finished event")
}

func TestAgentManagerEmitsMemoryCompactedRuntimeEvent(t *testing.T) {
	store := runtime.NewMemStore()
	eng := runtime.NewEngine(store)
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)
	created, err := mgr.CreateSession(createAgentSessionRequest{Operator: "tester", Language: "en"})
	require.NoError(t, err)
	sessionID := created.Session.SessionID

	const anchor = "memory-compaction-anchor"
	now := time.Now().UTC().Add(-time.Hour)
	mgr.mu.Lock()
	session := mgr.sessions[sessionID]
	session.Messages = nil
	for i := 0; i < agentSessionMaxMessages+5; i++ {
		role := "user"
		if i%2 == 1 {
			role = "agent"
		}
		content := "historical message"
		if i == 3 {
			content += " " + anchor
		}
		session.Messages = append(session.Messages, agentMessage{
			ID:        strings.Replace(runs.NewJobID(now.Add(time.Duration(i)*time.Second)), "job_", "msg_", 1),
			Role:      role,
			Content:   content,
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}
	session.ActiveTask = &agentTask{
		ID:        "task_memory_event",
		Goal:      "preserve long context",
		Kind:      "analysis",
		Status:    "running",
		CreatedAt: now,
		UpdatedAt: now,
	}
	mgr.mu.Unlock()

	_, err = mgr.SendMessage(context.Background(), sessionID, sendAgentMessageRequest{
		Operator: "tester",
		Language: "en",
		Message:  "continue",
	}, routerServices{})
	require.NoError(t, err)

	var compacted *runtime.Event
	for _, ev := range store.Events() {
		if ev.Type == "memory_compacted" {
			event := ev
			compacted = &event
			break
		}
	}
	if compacted == nil {
		t.Fatalf("expected memory_compacted event, got %#v", store.Events())
	}
	require.Equal(t, sessionID, compacted.Data["session_id"])
	require.Equal(t, "turn_count", compacted.Data["trigger"])
	require.EqualValues(t, agentSessionMemoryCompactionTurns, compacted.Data["trigger_threshold"])
	require.NotEmpty(t, compacted.TurnID)
	memory := compacted.Data["memory_summary"].(*agentMemorySummary)
	require.Contains(t, memory.Summary, anchor)
	require.Greater(t, compacted.Data["compacted_count"], 0)
}

func TestAgentManagerFinishRuntimeTurnPublishesTerminalEvents(t *testing.T) {
	store := runtime.NewMemStore()
	eng := runtime.NewEngine(store)
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	now := time.Now().UTC()
	sessionID := "agent_terminal_events"
	session := agentSession{
		SessionID: sessionID,
		ActiveTask: &agentTask{
			ID:        "task_terminal_events",
			Goal:      "terminal runtime events",
			Kind:      "chat",
			Status:    "running",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	userMessage := agentMessage{
		ID:        "msg_terminal_events",
		Role:      "user",
		Content:   "finish",
		Timestamp: now,
	}

	turnRef := mgr.beginRuntimeTurn(context.Background(), sessionID, "finish", "user", "read_only", session, userMessage)
	require.NotEmpty(t, turnRef.RunID)
	mgr.finishRuntimeTurn(context.Background(), sessionID, turnRef, sendAgentMessageResponse{
		SessionID:  sessionID,
		ActiveTask: &agentTask{ID: session.ActiveTask.ID, Status: "done", Summary: "done"},
		NextAction: &agentNextAction{Type: "finished", Reason: "done"},
	}, nil)

	var sawTurnFinished, sawTaskFinished bool
	for _, ev := range store.Events() {
		switch ev.Type {
		case "turn_finished":
			sawTurnFinished = true
			require.Equal(t, turnRef.RunID, ev.RunID)
			require.Equal(t, turnRef.TurnID, ev.TurnID)
			require.Equal(t, "finished", ev.Data["status"])
			require.Equal(t, "done", ev.Data["reason"])
		case "task_finished":
			sawTaskFinished = true
			require.Equal(t, turnRef.RunID, ev.RunID)
			require.Equal(t, session.ActiveTask.ID, ev.TaskID)
			require.Equal(t, sessionID, ev.Data["session_id"])
		}
	}
	require.True(t, sawTurnFinished, "expected turn_finished event")
	require.True(t, sawTaskFinished, "expected task_finished event")
}
