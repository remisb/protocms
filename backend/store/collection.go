package store

// Collection access for non-content records.
//
// Some datasets (notably the reserved _system dataset) hold records that are
// not schema-validated tenant content — users, API keys, and similar. These
// methods give callers a small, stable contract for those collections:
// snapshot, append-with-id, and replace, each correctly locked and persisted.
// Callers never touch the dataset's internal maps, lock, or id counter, so the
// internal representation can change without breaking them.

// CollectionItems returns a snapshot copy of the named collection's items. The
// copy is safe to read after the lock is released; mutating it does not affect
// stored state.
func (d *Dataset) CollectionItems(collection string) []ContentItem {
	d.mu.RLock()
	defer d.mu.RUnlock()
	items := d.content[collection]
	out := make([]ContentItem, len(items))
	copy(out, items)
	return out
}

// AppendCollectionItem appends one record to the named collection and persists,
// atomically. precondition runs first under the write lock against the current
// items; if it returns an error the append is aborted (e.g. a uniqueness
// check), so the check and the insert cannot race. makeItem receives the
// dataset's next id to stamp into the record. The stored item is returned on
// success. Both callbacks run under the write lock, so neither may call back
// into a locking Dataset method (it would self-deadlock; see MutateCollection).
func (d *Dataset) AppendCollectionItem(
	collection string,
	precondition func(items []ContentItem) error,
	makeItem func(id int) ContentItem,
) (ContentItem, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if precondition != nil {
		if err := precondition(d.content[collection]); err != nil {
			return nil, err
		}
	}
	item := makeItem(d.nextID)
	d.nextID++
	d.content[collection] = append(d.content[collection], item)
	d.persistLocked()
	return item, nil
}

// MutateCollection runs an atomic read-modify-write on the named collection.
// mutate receives the current items and returns the new slice plus a changed
// flag; the whole operation holds the write lock, so concurrent mutations
// cannot interleave (no lost updates). State is persisted only when changed is
// true; the returned changed flag is passed back to the caller.
//
// Two rules for the mutate callback:
//
//   - It must NOT mutate the passed-in items in place and then return
//     changed=false. The slice (and the ContentItem maps it holds, which are
//     reference types) is the live stored state, so an in-place edit takes
//     effect in memory immediately — but with changed=false it is never
//     persisted, leaving memory and disk diverged. To edit in place, build a
//     new slice (e.g. decode -> modify copies -> re-encode, as the system
//     store does) and return changed=true. Returning the original slice
//     unchanged with changed=false is the only safe no-op.
//   - It must NOT call back into any Dataset method that locks (CollectionItems,
//     AppendCollectionItem, the SystemStore accessors, etc.). The callback
//     already runs under d.mu's write lock, and sync.RWMutex is not reentrant,
//     so doing so self-deadlocks. Operate only on the passed-in items.
func (d *Dataset) MutateCollection(collection string, mutate func(items []ContentItem) (next []ContentItem, changed bool)) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	next, changed := mutate(d.content[collection])
	if changed {
		d.content[collection] = next
		d.persistLocked()
	}
	return changed
}
