package daemonws

import "context"

// MessageHandlerFunc handles a custom daemon message type.
// kind must use "rdev." prefix to avoid conflicts with upstream additions.
// payload is the raw JSON message body. Returns response bytes or nil for no response.
type MessageHandlerFunc func(ctx context.Context, identity ClientIdentity, payload []byte) ([]byte, error)

var messageHandlers = map[string]MessageHandlerFunc{}

// RegisterMessageHandler registers a custom daemon message handler.
// kind must begin with "rdev." (e.g. "rdev.file.tree").
// Duplicate registration for the same kind panics to prevent silent overwrites.
// Must be called before Hub starts (during init()).
func RegisterMessageHandler(kind string, fn MessageHandlerFunc) {
	if _, exists := messageHandlers[kind]; exists {
		panic("daemonws: duplicate message handler for kind: " + kind)
	}
	messageHandlers[kind] = fn
}

// dispatchExtension attempts to dispatch a message to a registered extension handler.
// Returns (response, true) if a handler was found; (nil, false) if not found.
func dispatchExtension(ctx context.Context, identity ClientIdentity, kind string, payload []byte) ([]byte, bool) {
	fn, ok := messageHandlers[kind]
	if !ok {
		return nil, false
	}
	resp, err := fn(ctx, identity, payload)
	if err != nil {
		return nil, true
	}
	return resp, true
}
