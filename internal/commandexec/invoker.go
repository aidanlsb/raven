package commandexec

import "context"

// ValidateFunc can normalize or reject a request before handler dispatch.
type ValidateFunc func(context.Context, Request) (Request, Result, bool)

// BeforeDispatchFunc runs after validation and handler lookup, immediately
// before dispatch. It may attach invocation-scoped state to the request or
// reject dispatch when a required precondition cannot be established.
type BeforeDispatchFunc func(context.Context, Request) (Request, Result, bool)

// AnnotateFunc post-processes a handler result before it is returned. It runs
// only for dispatched handlers (not validation/lookup failures) and is used to
// attach cross-cutting metadata such as the standard mutation-phase signal.
type AnnotateFunc func(context.Context, Request, Result) Result

// Invoker executes canonical Raven commands through a shared validation and
// dispatch pipeline.
type Invoker struct {
	handlers       *HandlerRegistry
	validate       ValidateFunc
	beforeDispatch BeforeDispatchFunc
	annotate       AnnotateFunc
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

// WithResultAnnotator sets an optional post-dispatch result annotator and
// returns the invoker for chaining.
func (i *Invoker) WithResultAnnotator(annotate AnnotateFunc) *Invoker {
	i.annotate = annotate
	return i
}

// WithBeforeDispatch sets an optional hook that runs immediately before the
// selected handler.
func (i *Invoker) WithBeforeDispatch(before BeforeDispatchFunc) *Invoker {
	i.beforeDispatch = before
	return i
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

	if i.beforeDispatch != nil {
		var result Result
		var ok bool
		req, result, ok = i.beforeDispatch(ctx, req)
		if !ok {
			return result
		}
	}
	result := handler(ctx, req)
	if i.annotate != nil {
		result = i.annotate(ctx, req, result)
	}
	return result
}

// Handlers exposes the registered handler set for migration-aware adapters.
func (i *Invoker) Handlers() *HandlerRegistry {
	return i.handlers
}
