// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Operations on the types declared in types.go. Living in a separate
// file forces the converter to associate methods with receivers
// declared in a sibling source file.
package fixture

// Add appends u to the repository and returns the new length.
func (r *Repo) Add(u User) int {
	r.Users = append(r.Users, u)
	return len(r.Users)
}

// Find returns the first user whose ID matches id, or the zero
// User and false when no entry is present.
func (r *Repo) Find(id int) (User, bool) {
	for _, u := range r.Users {
		if u.ID == id {
			return u, true
		}
	}
	return User{}, false
}
