package partialdata

import (
	"context"
	"errors"

	"github.com/cortexproject/cortex/pkg/querier/tripperware"
)

type Error struct{}

func (e Error) Error() string {
	return "partial data"
}

const ctxKey string = "partialDataCtxKey"

func ContextWithPartialData(ctx context.Context, isEnabled bool) context.Context {
	if isEnabled {
		return context.WithValue(ctx, ctxKey, true)
	}
	return ctx
}

func FromContext(ctx context.Context) bool {
	o := ctx.Value(ctxKey)
	if o == nil {
		return false
	}
	return o.(bool)
}

func IsEnabled(limits tripperware.Limits, userID string) bool {
	return limits.QueryPartialData(userID)
}

func ReturnPartialData(err error, isEnabled bool) bool {
	return isEnabled && errors.As(err, &Error{})
}
