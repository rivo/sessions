package sessions

import "testing"

// Test the transformation of the parent-parent list to the descendent list.
func TestRoleHierarchy(t *testing.T) {
	//      A     H
	//     /|\    |\
	//    B C D   I J
	//   /\   |      \
	//  E  F  G       K
	Persistence = ExtendablePersistenceLayer{
		RoleHierarchyFunc: func() (map[string]string, error) {
			return map[string]string{"B": "A", "C": "A", "D": "A", "E": "B", "F": "B", "G": "D", "I": "H", "J": "H", "K": "J"}, nil
		},
	}
	SetupRoleHierarchy()

	// Expected result.
	descendents := map[string][]string{
		"A": []string{"B", "C", "D", "E", "F", "G"},
		"B": []string{"E", "F"},
		"D": []string{"G"},
		"H": []string{"I", "J", "K"},
		"J": []string{"K"},
	}

	// Compare.
	in := func(needle string, haystack []string) bool {
		for _, s := range haystack {
			if s == needle {
				return true
			}
		}
		return false
	}
	if len(roles) != len(descendents) {
		t.Errorf("Users.roles has length %d, expected %d", len(roles), len(descendents))
	}
	for parent, d1 := range descendents {
		d2 := DescendentRoles(parent)
		if len(d1) != len(d2) {
			t.Errorf("Users.roles has %d descendents for %s (%s), expected %d (%s)", len(d2), parent, d2, len(d1), d1)
			continue
		}
		for _, d := range d1 {
			if !in(d, d2) {
				t.Errorf("Descendent %s not found in Users.roles for %s (%s)", d, parent, d2)
			}
		}
	}
}
