package commandexec

import "context"

type contextKey string

const invokerContextKey contextKey = "commandexec.invoker"

func withInvoker(ctx context.Context, invoker *Invoker) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, invokerContextKey, invoker)
}

// InvokerFromContext returns the canonical invoker bound to the execution context.
func InvokerFromContext(ctx context.Context) (*Invoker, bool) {
	if ctx == nil {
		return nil, false
	}
	invoker, ok := ctx.Value(invokerContextKey).(*Invoker)
	return invoker, ok && invoker != nil
}
