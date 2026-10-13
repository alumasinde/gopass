package tenancy

import (
	"context"
	"errors"
)

type key string

const Key key = "organization_id"

func With(ctx context.Context, id int64) context.Context { return context.WithValue(ctx, Key, id) }
func ID(ctx context.Context) (int64, error) {
	v, ok := ctx.Value(Key).(int64)
	if !ok || v < 1 {
		return 0, errors.New("organization context missing")
	}
	return v, nil
}
