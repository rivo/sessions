package sessions

import "sync"

var (
	// Maps a role to all of its descendent roles (excluding itself). If a role
	// is not contained, it has no descendent roles. Roles inherit the
	// capabilities of all of its descendent roles.
	roles map[string][]string

	// Synchronizes access to the roles map.
	roleMutex sync.RWMutex
)

// SetupRoleHierarchy initializes the role hierarchy.
func SetupRoleHierarchy() error {
	roleMutex.Lock()
	defer roleMutex.Unlock()

	// Load the role hierarchy and transform to descendent list.
	hierarchy, err := Persistence.RoleHierarchy()
	if err != nil {
		return err
	}
	roles = make(map[string][]string)
	touched := make(map[string]struct{})
	for child := range hierarchy {
		// Traverse upwards and touch. Add all untouched roles.
		var add []string
		parent, ok := hierarchy[child]
		for ok {
			if _, t := touched[child]; !t {
				add = append(add, child)
				touched[child] = struct{}{}
			}
			if len(add) > 0 {
				roles[parent] = append(roles[parent], add...)
			}
			child = parent
			parent, ok = hierarchy[child]
		}
	}

	return nil
}

// DescendentRoles returns all descendent roles of the given role, excluding
// the role itself.
func DescendentRoles(role string) []string {
	roleMutex.RLock()
	defer roleMutex.RUnlock()

	return roles[role]
}
