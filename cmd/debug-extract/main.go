package main

import (
  "context"
  "fmt"
  "os"

  "github.com/persistorai/persistor/internal/ingest"
)

func main() {
  path := "/home/brian/.openclaw/workspace/IDENTITY.md"
  if len(os.Args) > 1 {
    path = os.Args[1]
  }

  b, err := os.ReadFile(path)
  if err != nil {
    panic(err)
  }

  c := ingest.NewLLMClient()
  e := ingest.NewExtractor(c)
  r, err := e.Extract(context.Background(), string(b))
  if err != nil {
    panic(err)
  }

  fmt.Printf("entities=%d rels=%d facts=%d\n", len(r.Entities), len(r.Relationships), len(r.Facts))
  fmt.Printf("entities: %#v\n", r.Entities)
  fmt.Printf("relationships: %#v\n", r.Relationships)
  fmt.Printf("facts: %#v\n", r.Facts)
}
