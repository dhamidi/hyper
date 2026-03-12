package hyper

// FindLink returns the first Link with the given rel, or false if not found.
func FindLink(rep Representation, rel string) (Link, bool) {
	for _, l := range rep.Links {
		if l.Rel == rel {
			return l, true
		}
	}
	return Link{}, false
}

// FindAction returns the first Action with the given rel, or false if not found.
func FindAction(rep Representation, rel string) (Action, bool) {
	for _, a := range rep.Actions {
		if a.Rel == rel {
			return a, true
		}
	}
	return Action{}, false
}

// FindEmbedded returns the embedded representations in the given slot, or nil.
func FindEmbedded(rep Representation, slot string) []Representation {
	if rep.Embedded == nil {
		return nil
	}
	return rep.Embedded[slot]
}

// ActionValues extracts default values from an Action's Fields as a map.
// This is useful for pre-populating form values before user overrides.
func ActionValues(action Action) map[string]any {
	result := make(map[string]any)
	for _, f := range action.Fields {
		if f.Value != nil {
			result[f.Name] = f.Value
		}
	}
	return result
}
