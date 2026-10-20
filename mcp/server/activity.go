package server

// activityIDKey carries only a server-assigned correlation label from an
// accepted agent run into the shared gate. It is not a permission, continuation
// locator or durable-history handle; no transport reads it from client input.
type activityIDKey struct{}
