//go:build !linux

package native

import "context"

func capture(context.Context, Config) (Result, error) {
	return Result{}, ErrUnsupported
}
