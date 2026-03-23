package commandexec

import "context"

// ValidateFunc can normalize or reject a request before handler dispatch.
type ValidateFunc func(context.Context, Request) (Request, Result, bool)

// Invoker executes canonical Raven commands through a shared validation and
// dispatch pipeline.
type Invoker struct {
	handlers *HandlerRegistry
	validate ValidateFunc
}

// NewInvoker constructs an invoker with the provided registry and validator.
func NewInvoker(handlers *HandlerRegistry, validate ValidateFunc) *Invoker {
	if handlers == nil {
		handlers = NewHandlerRegistry()
	}
	return &Invoker{
		handlers: handlers,
		validate: validate,
	}
}

// Execute validates and dispatches a command request.
func (i *Invoker) Execute(ctx context.Context, req Request) Result {
	ctx = withInvoker(ctx, i)

	if i.validate != nil {
		validated, result, ok := i.validate(ctx, req)
		if !ok {
			return result
		}
		req = validated
	}

	handler, ok := i.handlers.Lookup(req.CommandID)
	if !ok {
		return Failure(
			"COMMAND_NOT_FOUND",
			"unknown command: "+req.CommandID,
			map[string]interface{}{"command": req.CommandID},
			"Choose a registered command and retry",
		)
	}

	return handler(ctx, req)
}

// Handlers exposes the registered handler set for migration-aware adapters.
func (i *Invoker) Handlers() *HandlerRegistry {
	return i.handlers
}
