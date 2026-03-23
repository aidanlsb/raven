package commandexec

import (
	"context"
	"sync"
)

// Handler executes one canonical command.
type Handler func(context.Context, Request) Result

// HandlerRegistry stores handlers by canonical command ID.
type HandlerRegistry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

// NewHandlerRegistry creates an empty handler registry.
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[string]Handler),
	}
}

// Register binds a handler to a command ID.
func (r *HandlerRegistry) Register(commandID string, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.handlers == nil {
		r.handlers = make(map[string]Handler)
	}
	r.handlers[commandID] = handler
}

// Lookup returns the handler registered for a command ID.
func (r *HandlerRegistry) Lookup(commandID string) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handler, ok := r.handlers[commandID]
	return handler, ok
}

// Handlers returns the underlying registry for read-only inspection.
func (r *HandlerRegistry) Handlers() map[string]Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[string]Handler, len(r.handlers))
	for commandID, handler := range r.handlers {
		out[commandID] = handler
	}
	return out
}
