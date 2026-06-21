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
// success.
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
// true. The bool mutate ultimately reports (e.g. "found and updated") is
// returned to the caller.
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
