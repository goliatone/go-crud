package cms_delivery

import (
	"os"
	"strings"
	"testing"
)

func TestDeliverySchemaQueries(t *testing.T) {
	data, err := os.ReadFile("./output/schema.graphql")
	if err != nil {
		t.Fatalf("read schema.graphql: %v", err)
	}
	schema := string(data)
	expected := []string{
		"getContent",
		"listContent",
		"getPage",
		"listPage",
		"getMenu",
		"listMenu",
	}
	for _, name := range expected {
		if !strings.Contains(schema, name) {
			t.Fatalf("expected query %q in schema", name)
		}
	}
}
