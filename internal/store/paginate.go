package store

import (
	"github.com/araki/pibench/internal/cursor"
	"github.com/araki/pibench/internal/model"
)

// BuildPage assembles a query page from points fetched in ascending (ts,member)
// order for the window starting at the cursor lower bound. Callers fetch up to
// limit+1 points (offset by curSkip at curTs); BuildPage trims to limit and, if
// more remain, emits an opaque keyset cursor.
//
// curTs/curSkip describe the lower bound the fetch started from so the same-ts
// skip count accumulates correctly across page boundaries.
func BuildPage(fetched []model.Point, limit int, curTs int64, curSkip int) (page []model.Point, next string) {
	if len(fetched) <= limit {
		return fetched, ""
	}
	page = fetched[:limit]
	lastTs := page[limit-1].TS

	run := 0
	for i := limit - 1; i >= 0 && page[i].TS == lastTs; i-- {
		run++
	}
	skip := run
	if lastTs == curTs {
		skip += curSkip
	}
	return page, cursor.Encode(lastTs, skip)
}
