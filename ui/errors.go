package ui

import (
	"context"
	"errors"

	"github.com/xmhrnx/giode"
)

// isCanceled reports a picker or dialog the user closed.
func isCanceled(err error) bool {
	return errors.Is(err, giode.ErrCanceled) || errors.Is(err, context.Canceled)
}
