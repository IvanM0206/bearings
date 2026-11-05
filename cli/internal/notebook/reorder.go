package notebook

// Reordering moves an item one place up or down among its siblings. When
// the two neighbours have different `order` values their values are
// swapped (two writes). When they are equal, which happens after manual
// edits, the siblings are renumbered 0..n-1 so that the result is
// unambiguous; only files whose value changes are written. Every file is
// replaced atomically with conflict detection, but the operation as a
// whole is not transactional: if a write fails, the error is returned and
// the siblings written so far keep their new order.

// ReorderSection moves a section one place up (delta < 0) or down
// (delta > 0). It reports whether anything moved: a section already first
// (or last) stays where it is.
func (nb *Notebook) ReorderSection(slug string, delta int) (bool, error) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	if !ValidSlug(slug) {
		return false, newErr(CodeInvalidSlug, "", "")
	}
	secs, err := nb.Sections()
	if err != nil {
		return false, err
	}
	idx := -1
	for i, s := range secs {
		if s.Slug == slug {
			idx = i
		}
	}
	if idx < 0 {
		return false, newErr(CodeNotFound, sectionDirRel(slug), "")
	}
	j := idx + sign(delta)
	if delta == 0 || j < 0 || j >= len(secs) {
		return false, nil
	}
	orders := make([]int, len(secs))
	for i, s := range secs {
		orders[i] = s.Order
	}
	secs[idx], secs[j] = secs[j], secs[idx]
	newOrders := reorderValues(orders, idx, j, len(secs))
	for i, s := range secs {
		if s.Order == newOrders[i] {
			continue
		}
		s.Order = newOrders[i]
		s.Updated = nb.touch(s.Created)
		v := s.Version
		if _, err := nb.writeSection(s, &v); err != nil {
			return false, err
		}
	}
	return true, nil
}

// ReorderTopic moves a topic one place up (delta < 0) or down (delta > 0)
// inside its section. It reports whether anything moved.
func (nb *Notebook) ReorderTopic(ref TopicRef, delta int) (bool, error) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	if !ref.valid() {
		return false, newErr(CodeInvalidRef, "", "")
	}
	if _, err := nb.loadSection(ref.Section); err != nil {
		return false, err
	}
	ts, err := nb.topicsOf(ref.Section)
	if err != nil {
		return false, err
	}
	idx := -1
	for i, t := range ts {
		if t.Slug == ref.Topic {
			idx = i
		}
	}
	if idx < 0 {
		return false, newErr(CodeNotFound, topicRel(ref), "")
	}
	j := idx + sign(delta)
	if delta == 0 || j < 0 || j >= len(ts) {
		return false, nil
	}
	orders := make([]int, len(ts))
	for i, t := range ts {
		orders[i] = t.Order
	}
	ts[idx], ts[j] = ts[j], ts[idx]
	newOrders := reorderValues(orders, idx, j, len(ts))
	for i, t := range ts {
		if t.Order == newOrders[i] {
			continue
		}
		t.Order = newOrders[i]
		t.Updated = nb.touch(t.Created)
		v := t.Version
		if _, err := nb.writeTopic(t, &v); err != nil {
			return false, err
		}
	}
	return true, nil
}

// reorderValues returns the `order` value for each position after the items
// at positions i and j were swapped. orders holds the values of the
// positions before the swap. When all the values are strictly increasing, the positions keep their values (so the two items trade
// theirs); otherwise everything is renumbered 0..n-1.
func reorderValues(orders []int, i, j, n int) []int {
	out := make([]int, n)
	if strictlyIncreasing(orders) {
		copy(out, orders)
		return out
	}
	for k := range out {
		out[k] = k
	}
	return out
}

func strictlyIncreasing(v []int) bool {
	for i := 1; i < len(v); i++ {
		if v[i] <= v[i-1] {
			return false
		}
	}
	return true
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
