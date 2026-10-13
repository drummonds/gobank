package api

import "testing"

// The document parses as OpenAPI 3 and lists every operation with its
// method and path, which the routers' tests walk.
func TestDocumentListsItsOperations(t *testing.T) {
	ops, err := Operations()
	if err != nil {
		t.Fatal(err)
	}
	want := map[Operation]bool{
		{"GET", "/v1/health"}:       true,
		{"GET", "/v1/openapi.json"}: true,
		{"POST", "/v1/login"}:       true,
		{"GET", "/about.json"}:      true,
		{"POST", "/settings"}:       true,
	}
	for _, op := range ops {
		delete(want, op)
	}
	if len(want) > 0 {
		t.Errorf("operations missing from the document: %v (have %v)", want, ops)
	}
	for _, op := range ops {
		if op.Method != "GET" && op.Method != "POST" {
			t.Errorf("%v: the surface is GET and POST only", op)
		}
	}
}
