package core

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunWithTimeout(t *testing.T) {
	testCases := []struct {
		desc        string
		timeout     time.Duration
		fn          func() error
		expectError bool
		errContains string
	}{
		{
			desc:    "function completes before timeout",
			timeout: time.Second,
			fn: func() error {
				return nil
			},
		},
		{
			desc:    "function returns error before timeout",
			timeout: time.Second,
			fn: func() error {
				return errors.New("plugin error")
			},
			expectError: true,
			errContains: "plugin error",
		},
		{
			desc:    "function blocks and gets timed out",
			timeout: 100 * time.Millisecond,
			fn: func() error {
				select {}
			},
			expectError: true,
			errContains: "timed out after",
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			err := runWithTimeout(test.timeout, test.fn)

			if test.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
