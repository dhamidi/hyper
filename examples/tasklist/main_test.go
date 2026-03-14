package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testServer(t *testing.T) (*httptest.Server, *TaskStore) {
	t.Helper()
	store := NewTaskStore()
	store.Create("Test task one")
	store.Create("Test task two")
	srv := httptest.NewServer(newMux(store))
	t.Cleanup(srv.Close)
	return srv, store
}

func doReq(t *testing.T, method, url, accept, body string) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatal(err)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestListTasks_HTML(t *testing.T) {
	srv, _ := testServer(t)

	resp := doReq(t, "GET", srv.URL+"/", "text/html", "")
	body := readBody(t, resp)

	if resp.StatusCode != 200 {
		t.Fatalf("got status %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("got content-type %q, want text/html", ct)
	}
	if !strings.Contains(body, "<article") {
		t.Error("response does not contain <article")
	}
	if !strings.Contains(body, "Test task one") {
		t.Error("response does not contain 'Test task one'")
	}
	if !strings.Contains(body, "Test task two") {
		t.Error("response does not contain 'Test task two'")
	}
}

func TestListTasks_JSON(t *testing.T) {
	srv, _ := testServer(t)

	resp := doReq(t, "GET", srv.URL+"/", "application/json", "")
	body := readBody(t, resp)

	if resp.StatusCode != 200 {
		t.Fatalf("got status %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("got content-type %q, want application/json", ct)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result["kind"] != "task-list" {
		t.Errorf("got kind %q, want task-list", result["kind"])
	}
	embedded, ok := result["embedded"].(map[string]any)
	if !ok {
		t.Fatal("missing embedded field")
	}
	items, ok := embedded["items"].([]any)
	if !ok {
		t.Fatal("missing embedded items")
	}
	if len(items) != 2 {
		t.Errorf("got %d items, want 2", len(items))
	}
}

func TestCreateTask(t *testing.T) {
	srv, _ := testServer(t)

	resp := doReq(t, "POST", srv.URL+"/tasks", "application/json", "title=New+Task")
	body := readBody(t, resp)

	if resp.StatusCode != 201 {
		t.Fatalf("got status %d, want 201", resp.StatusCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result["kind"] != "task" {
		t.Errorf("got kind %q, want task", result["kind"])
	}

	// Verify task appears in list
	resp2 := doReq(t, "GET", srv.URL+"/", "application/json", "")
	body2 := readBody(t, resp2)
	if !strings.Contains(body2, "New Task") {
		t.Error("created task not found in list")
	}
}

func TestToggleTask(t *testing.T) {
	srv, _ := testServer(t)

	resp := doReq(t, "POST", srv.URL+"/tasks/1/toggle", "application/json", "")
	body := readBody(t, resp)

	if resp.StatusCode != 200 {
		t.Fatalf("got status %d, want 200", resp.StatusCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	state, ok := result["state"].(map[string]any)
	if !ok {
		t.Fatal("missing state")
	}
	if state["status"] != "done" {
		t.Errorf("got status %q, want done", state["status"])
	}

	// Toggle back
	resp2 := doReq(t, "POST", srv.URL+"/tasks/1/toggle", "application/json", "")
	body2 := readBody(t, resp2)
	var result2 map[string]any
	json.Unmarshal([]byte(body2), &result2)
	state2 := result2["state"].(map[string]any)
	if state2["status"] != "pending" {
		t.Errorf("got status %q, want pending after second toggle", state2["status"])
	}
}

func TestDeleteTask(t *testing.T) {
	srv, _ := testServer(t)

	resp := doReq(t, "DELETE", srv.URL+"/tasks/1", "application/json", "")
	resp.Body.Close()

	if resp.StatusCode != 204 {
		t.Fatalf("got status %d, want 204", resp.StatusCode)
	}

	// Verify task is gone
	resp2 := doReq(t, "GET", srv.URL+"/", "application/json", "")
	body2 := readBody(t, resp2)
	if strings.Contains(body2, "Test task one") {
		t.Error("deleted task still in list")
	}
}

func TestCreateTask_EmptyTitle(t *testing.T) {
	srv, _ := testServer(t)

	resp := doReq(t, "POST", srv.URL+"/tasks", "application/json", "title=")
	readBody(t, resp)

	if resp.StatusCode != 422 {
		t.Fatalf("got status %d, want 422", resp.StatusCode)
	}
}

func TestCreateForm_HasAction(t *testing.T) {
	srv, _ := testServer(t)
	resp := doReq(t, "GET", srv.URL+"/", "text/html", "")
	body := readBody(t, resp)
	if !strings.Contains(body, `action="/tasks"`) {
		t.Errorf("create form should have action=\"/tasks\", got body:\n%s", body)
	}
}

func TestContentNegotiation(t *testing.T) {
	srv, _ := testServer(t)

	// HTML
	respHTML := doReq(t, "GET", srv.URL+"/", "text/html", "")
	readBody(t, respHTML)
	if ct := respHTML.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("HTML: got content-type %q", ct)
	}

	// JSON
	respJSON := doReq(t, "GET", srv.URL+"/", "application/json", "")
	readBody(t, respJSON)
	if ct := respJSON.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("JSON: got content-type %q", ct)
	}
}
