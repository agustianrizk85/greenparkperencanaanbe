// Package repository defines storage access for the Perencanaan dashboard and
// ships a concurrency-safe, writable in-memory implementation seeded with
// representative data. Replacing it with a database-backed store only requires
// providing the same set of methods used by the service layer.
package repository

import "errors"

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("resource not found")
