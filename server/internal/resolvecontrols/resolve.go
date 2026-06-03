// Package resolvecontrols filters proposed control mappings against the seeded
// controls catalog in Postgres. It is the database-coupled half of control
// mapping: the pure rule evaluation lives in the open classify package, and
// this resolve step (a single query) stays server side.
package resolvecontrols

import (
	"context"
	"encoding/json"

	"github.com/stonewrit/stonewrit/classify"
	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
)

// ResolveAgainstCatalog drops mappings whose (framework, control_id) is not
// seeded in the controls table. Pass any Queries instance; it works inside a
// transaction by passing a tx-wrapped Queries.
func ResolveAgainstCatalog(ctx context.Context, q *queries.Queries, proposed []classify.Mapping) ([]classify.Mapping, error) {
	if len(proposed) == 0 {
		return proposed, nil
	}

	type refRow struct {
		Framework string `json:"framework"`
		ControlID string `json:"control_id"`
	}
	rows := make([]refRow, len(proposed))
	for i, m := range proposed {
		rows[i] = refRow{Framework: m.Framework, ControlID: m.ControlID}
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}

	resolved, err := q.ResolveControlReferences(ctx, payload)
	if err != nil {
		return nil, err
	}

	allowed := make(map[string]struct{}, len(resolved))
	for _, r := range resolved {
		allowed[r.Framework+":"+r.ControlID] = struct{}{}
	}

	kept := proposed[:0]
	for _, m := range proposed {
		if _, ok := allowed[m.Framework+":"+m.ControlID]; ok {
			kept = append(kept, m)
		}
	}
	return kept, nil
}
