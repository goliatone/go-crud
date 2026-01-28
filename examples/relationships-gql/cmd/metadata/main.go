package main

import (
	"flag"
	"log"
	"os"

	"github.com/goliatone/go-crud"
	_ "github.com/goliatone/go-crud/examples/relationships-gql/registrar"
)

func main() {
	out := flag.String("out", "metadata.json", "output path for registry snapshot")
	flag.Parse()

	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("open %s: %v", *out, err)
	}
	defer f.Close()

	if err := crud.ExportSchemas(f, crud.WithSchemaExportIndent("  ")); err != nil {
		log.Fatalf("encode schemas: %v", err)
	}
}
