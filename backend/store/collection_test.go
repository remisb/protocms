package store

import (
	"errors"
	"sync"
	"testing"
)

// makeItem stamps the assigned id; precondition is checked under the same lock.
func appendN(d *Dataset, coll string, n int) {
	d.AppendCollectionItem(coll, nil, func(id int) ContentItem {
		return ContentItem{"id": id, "n": n}
	})
}

func TestAppendCollectionItemAssignsDistinctIDs(t *testing.T) {
	t.Chdir(t.TempDir())
	d := NewRegistry().Load("_concurrent")

	const workers = 50
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(n int) { defer wg.Done(); appendN(d, "things", n) }(i)
	}
	wg.Wait()

	items := d.CollectionItems("things")
	if len(items) != workers {
		t.Fatalf("got %d items, want %d (lost append under concurrency)", len(items), workers)
	}
	seen := map[int]bool{}
	for _, it := range items {
		id := toInt(it["id"])
		if seen[id] {
			t.Fatalf("duplicate id %d assigned under concurrency", id)
		}
		seen[id] = true
	}
}

// toInt local to the test (the production one was removed in the dedup refactor).
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

func TestAppendCollectionItemPreconditionAborts(t *testing.T) {
	t.Chdir(t.TempDir())
	d := NewRegistry().Load("_precond")

	sentinel := errors.New("nope")
	_, err := d.AppendCollectionItem("things",
		func(items []ContentItem) error { return sentinel },
		func(id int) ContentItem { return ContentItem{"id": id} },
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("got err %v, want sentinel", err)
	}
	if got := len(d.CollectionItems("things")); got != 0 {
		t.Fatalf("precondition failure still appended (%d items)", got)
	}
}

func TestMutateCollectionPersistsOnlyWhenChanged(t *testing.T) {
	t.Chdir(t.TempDir())
	d := NewRegistry().Load("_mutate")
	appendN(d, "things", 1)

	// changed=false must not alter state.
	changed := d.MutateCollection("things", func(items []ContentItem) ([]ContentItem, bool) {
		return nil, false
	})
	if changed {
		t.Fatal("MutateCollection reported changed for a no-op")
	}
	if got := len(d.CollectionItems("things")); got != 1 {
		t.Fatalf("no-op mutate altered state: %d items", got)
	}
}
