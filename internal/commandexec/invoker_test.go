package commandexec

import (
	"context"
	"testing"
)

func TestInvokerExecuteCallsRegisteredHandler(t *testing.T) {
	registry := NewHandlerRegistry()
	registry.Register("new", func(_ context.Context, req Request) Result {
		return Success(map[string]any{
			"command": req.CommandID,
			"caller":  req.Caller,
		}, &Meta{Count: 1})
	})

	invoker := NewInvoker(registry, nil)
	result := invoker.Execute(context.Background(), Request{
		CommandID: "new",
		Caller:    CallerCLI,
	})

	if !result.OK {
		t.Fatalf("result.OK = false, want true: %#v", result)
	}
	if result.Error != nil {
		t.Fatalf("result.Error = %#v, want nil", result.Error)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("result.Data type = %T, want map[string]any", result.Data)
	}
	if got := data["command"]; got != "new" {
		t.Fatalf("data[command] = %v, want new", got)
	}
	if got := data["caller"]; got != CallerCLI {
		t.Fatalf("data[caller] = %v, want %v", got, CallerCLI)
	}
	if result.Meta == nil || result.Meta.Count != 1 {
		t.Fatalf("result.Meta = %#v, want Count=1", result.Meta)
	}
}

func TestInvokerExecuteRunsValidatorBeforeDispatch(t *testing.T) {
	registry := NewHandlerRegistry()
	registry.Register("new", func(_ context.Context, req Request) Result {
		return Success(req.Args, nil)
	})

	validatorCalled := false
	invoker := NewInvoker(registry, func(_ context.Context, req Request) (Request, Result, bool) {
		validatorCalled = true
		req.Args = map[string]any{"title": "Freya"}
		return req, Result{}, true
	})

	result := invoker.Execute(context.Background(), Request{CommandID: "new"})

	if !validatorCalled {
		t.Fatal("validator was not called")
	}
	if !result.OK {
		t.Fatalf("result.OK = false, want true: %#v", result)
	}
	args, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("result.Data type = %T, want map[string]any", result.Data)
	}
	if got := args["title"]; got != "Freya" {
		t.Fatalf("args[title] = %v, want Freya", got)
	}
}

func TestInvokerExecuteReturnsValidationFailure(t *testing.T) {
	invoker := NewInvoker(NewHandlerRegistry(), func(_ context.Context, req Request) (Request, Result, bool) {
		return req, Failure("INVALID_ARGS", "missing title", nil, "Provide title"), false
	})

	result := invoker.Execute(context.Background(), Request{CommandID: "new"})

	if result.OK {
		t.Fatalf("result.OK = true, want false: %#v", result)
	}
	if result.Error == nil || result.Error.Code != "INVALID_ARGS" {
		t.Fatalf("result.Error = %#v, want INVALID_ARGS", result.Error)
	}
}

func TestInvokerExecuteReturnsCommandNotFound(t *testing.T) {
	invoker := NewInvoker(nil, nil)

	result := invoker.Execute(context.Background(), Request{CommandID: "missing"})

	if result.OK {
		t.Fatalf("result.OK = true, want false: %#v", result)
	}
	if result.Error == nil || result.Error.Code != "COMMAND_NOT_FOUND" {
		t.Fatalf("result.Error = %#v, want COMMAND_NOT_FOUND", result.Error)
	}
}
