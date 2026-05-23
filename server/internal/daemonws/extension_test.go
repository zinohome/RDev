package daemonws

import (
	"context"
	"testing"
)

func TestRegisterMessageHandler(t *testing.T) {
	messageHandlers = map[string]MessageHandlerFunc{}

	called := false
	RegisterMessageHandler("rdev.test.ping", func(ctx context.Context, identity ClientIdentity, payload []byte) ([]byte, error) {
		called = true
		return []byte(`{"pong":true}`), nil
	})

	resp, handled := dispatchExtension(context.Background(), ClientIdentity{}, "rdev.test.ping", nil)
	if !handled {
		t.Fatal("expected handler to be called")
	}
	if !called {
		t.Fatal("handler was not invoked")
	}
	if string(resp) != `{"pong":true}` {
		t.Errorf("unexpected response: %s", resp)
	}
}

func TestDispatchExtension_UnknownKind(t *testing.T) {
	messageHandlers = map[string]MessageHandlerFunc{}

	_, handled := dispatchExtension(context.Background(), ClientIdentity{}, "unknown.kind", nil)
	if handled {
		t.Fatal("expected unhandled for unknown kind")
	}
}

func TestRegisterMessageHandler_DuplicatePanics(t *testing.T) {
	messageHandlers = map[string]MessageHandlerFunc{}

	RegisterMessageHandler("rdev.dup", func(_ context.Context, _ ClientIdentity, _ []byte) ([]byte, error) {
		return nil, nil
	})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	RegisterMessageHandler("rdev.dup", func(_ context.Context, _ ClientIdentity, _ []byte) ([]byte, error) {
		return nil, nil
	})
}
