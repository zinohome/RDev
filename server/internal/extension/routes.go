package extension

import "github.com/go-chi/chi/v5"

var routeRegistrars []func(chi.Router)

// RegisterExtensionRoutes registers a route mount function.
// Must be called during init() or before BuildRouter() executes.
func RegisterExtensionRoutes(fn func(chi.Router)) {
	routeRegistrars = append(routeRegistrars, fn)
}

// MountAll mounts all registered extension routes onto the given router.
// Called by BuildRouter after all core routes are registered.
func MountAll(r chi.Router) {
	for _, fn := range routeRegistrars {
		fn(r)
	}
}
