package cms_management

import (
	"os"
	"strings"
	"testing"
)

func TestManagementSchemaQueriesAndMutations(t *testing.T) {
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
		"getContentType",
		"listContentType",
		"createContent",
		"updateContent",
		"deleteContent",
		"createPage",
		"updatePage",
		"deletePage",
		"createContentType",
		"updateContentType",
		"deleteContentType",
	}
	for _, name := range expected {
		if !strings.Contains(schema, name) {
			t.Fatalf("expected operation %q in schema", name)
		}
	}
}
