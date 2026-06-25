package db

import (
	"fmt"
	"strings"
)

// hiddenItemIDs lists item IDs that are excluded from all item queries.
// These are typically test/placeholder rows from the database source that
// should not be visible to users. Add new IDs here as they are identified.
var hiddenItemIDs = []int{
	11400, // test item from database creator
}

// hiddenItemSet is a fast lookup version of hiddenItemIDs.
var hiddenItemSet = func() map[int]bool {
	m := make(map[int]bool, len(hiddenItemIDs))
	for _, id := range hiddenItemIDs {
		m[id] = true
	}
	return m
}()

// isHiddenItem reports whether id is on the hidden list.
func isHiddenItem(id int) bool {
	return hiddenItemSet[id]
}

// hiddenItemClause returns a SQL fragment and args to exclude all hidden items.
// Returns an empty string and nil args if there are no hidden IDs.
func hiddenItemClause() (string, []any) {
	if len(hiddenItemIDs) == 0 {
		return "", nil
	}
	placeholders := strings.Repeat("?,", len(hiddenItemIDs))
	placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
	clause := fmt.Sprintf("id NOT IN (%s)", placeholders)
	args := make([]any, len(hiddenItemIDs))
	for i, id := range hiddenItemIDs {
		args[i] = id
	}
	return clause, args
}
