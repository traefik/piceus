package core

import (
	"context"

	"github.com/stealthrocket/wasi-go"
	"github.com/stealthrocket/wasi-go/imports"
	wazergo_wasip1 "github.com/stealthrocket/wasi-go/imports/wasi_snapshot_preview1"
	"github.com/stealthrocket/wazergo"
	"github.com/tetratelabs/wazero"
	wazero_wasip1 "github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func instantiate(ctx context.Context, runtime wazero.Runtime, mod wazero.CompiledModule) (context.Context, error) {
	extension := imports.DetectSocketsExtension(mod)
	if extension != nil {
		hostModule := wazergo_wasip1.NewHostModule(*extension)

		builder := imports.NewBuilder().WithSocketsExtension("auto", mod)

		var (
			sys wasi.System
			err error
		)

		ctx, sys, err = builder.Instantiate(ctx, runtime)
		if err != nil {
			return nil, err
		}

		_, err = wazergo.Instantiate(ctx, runtime, hostModule, wazergo_wasip1.WithWASI(sys))
		if err != nil {
			return nil, err
		}

		return ctx, nil
	}

	_, err := wazero_wasip1.Instantiate(ctx, runtime)
	if err != nil {
		return nil, err
	}

	return ctx, nil
}
