package entity

import "errors"

// ErrInsufficientStorage means a blob does not fit in the free space of the
// storage filesystem, so it was refused before any byte was written.
var ErrInsufficientStorage = errors.New("insufficient storage")
