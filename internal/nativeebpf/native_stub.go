//go:build !linux

package nativeebpf

import (
	"context"
	"fmt"
)

func record(_ context.Context, _ Options) (Result, error) {
	return Result{}, fmt.Errorf("native eBPF recorder is only supported on Linux")
}
