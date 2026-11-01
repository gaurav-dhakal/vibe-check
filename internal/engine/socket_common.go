package engine

import "errors"

// errNoSocket keeps the "not found" wording in one place across platforms.
var errNoSocket = errors.New("no container runtime endpoint found")
