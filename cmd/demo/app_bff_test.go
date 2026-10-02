//go:build !(js && wasm)

package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The BFF mounted in the demo logs a real customer in with the app
// password and serves that customer's own figures (ADR-0002 stage 1).
func TestAppBFFServesRealCustomer(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	ts := httptest.NewServer(newAppBFF(ds, "letmein", slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(ts.Close)

	post := func(body string) (int, map[string]any) {
		res, err := http.Post(ts.URL+"/v1/login", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var m map[string]any
		json.NewDecoder(res.Body).Decode(&m)
		return res.StatusCode, m
	}
	if code, _ := post(`{"customer_id":"cust-001","password":"wrong"}`); code != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", code)
	}
	code, m := post(`{"customer_id":"cust-001","password":"letmein"}`)
	if code != http.StatusOK {
		t.Fatalf("login: %d %v", code, m)
	}
	token, _ := m["token"].(string)
	screen, _ := m["screen"].(map[string]any)
	if token == "" || screen["title"] != ds.lookupName("cust-001") {
		t.Fatalf("login should return a token and the customer's own accounts screen: %v", m)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/screen/activity", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var activity map[string]any
	json.NewDecoder(res.Body).Decode(&activity)
	if res.StatusCode != http.StatusOK || activity["id"] != "activity" {
		t.Fatalf("activity: %d %v", res.StatusCode, activity)
	}
	sawTx := false
	for _, c := range activity["body"].([]any) {
		if c.(map[string]any)["type"] == "tx" {
			sawTx = true
		}
	}
	if !sawTx {
		t.Error("the funded customer's activity shows no transactions")
	}
}
