# Getting Started

Collect a week of Claude Code activity and persist it to the local store:

```go
package main

import (
	"context"
	"fmt"
	"time"

	omnidevx "github.com/plexusone/omnidevx-core"
	"github.com/plexusone/omnidevx-core/providers/claudecode"
	"github.com/plexusone/omnidevx-core/store"
)

func main() {
	ctx := context.Background()

	collector, err := claudecode.New(claudecode.Options{})
	if err != nil {
		panic(err)
	}

	result, err := collector.Collect(ctx, omnidevx.CollectRequest{
		Period: omnidevx.Period{
			Start: time.Now().AddDate(0, 0, -7),
		},
		Subject: omnidevx.SubjectRef{PersonID: "person:jane"},
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("collected %d events (%d diagnostics)\n",
		len(result.Events), len(result.Diagnostics))

	s, err := store.Open(store.Options{}) // ~/.plexusone/omnidevx/data
	if err != nil {
		panic(err)
	}
	written, err := s.Write(ctx, result.Events)
	if err != nil {
		panic(err)
	}
	fmt.Printf("stored %d new, %d duplicates\n",
		written.Written, written.Duplicates)
}
```

Re-running is safe: deterministic event IDs make writes idempotent.

Read events back for analysis:

```go
read, err := s.Read(ctx, store.Query{
	Period:  omnidevx.Period{Start: weekStart, End: weekEnd},
	Product: "claude-code", // optional source filter
})
```

For multi-collector composition (Claude Code + Codex + git + GitHub) see
the batteries-included [`omnidevx`](https://github.com/plexusone/omnidevx)
module and its `Engine`.
