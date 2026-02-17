package persistence

import "github.com/thwoodle/open-pilot/internal/core/session"

// Snapshot alias keeps sqlite persistence package explicit about boundary types.
type Snapshot = session.Snapshot
