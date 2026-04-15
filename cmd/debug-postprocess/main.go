package main

import (
  "fmt"
  "os"

  "github.com/persistorai/persistor/internal/ingest"
)

func main() {
  result := &ingest.ExtractionResult{
    Entities: []ingest.ExtractedEntity{
      {Name: "Brian", Type: "person"},
      {Name: "Brian Colinger", Type: "person"},
      {Name: "Scout", Type: "service"},
    },
    Relationships: []ingest.ExtractedRelationship{
      {Source: "Brian", Target: "Scout", Relation: "uses", Confidence: 0.9},
      {Source: "Brian Colinger", Target: "Scout", Relation: "works_on", Confidence: 0.9},
      {Source: "Scout", Target: "Brian", Relation: "partners_with", Confidence: 0.9},
    },
    Facts: []ingest.ExtractedFact{
      {Subject: "Brian", Property: "role", Value: "engineer"},
    },
  }

  out := ingest.PostProcessExtraction(result, []string{"Brian Colinger", "Scout"})
  fmt.Fprintf(os.Stdout, "%#v\n", out)
}
