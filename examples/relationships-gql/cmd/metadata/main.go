package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/goliatone/go-crud"
	_ "github.com/goliatone/go-crud/examples/relationships-gql/registrar"
)

func main() {
	out := flag.String("out", "metadata.json", "output path for registry snapshot")
	flag.Parse()

	entries := crud.ListSchemas()
	if len(entries) == 0 {
		log.Fatal("no schemas registered; ensure registrar init ran")
	}

	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("open %s: %v", *out, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(entries); err != nil {
		log.Fatalf("encode schemas: %v", err)
	}
}
