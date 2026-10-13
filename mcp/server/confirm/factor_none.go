package confirm

import (
	"context"
	"fmt"
)

// refusingFactor is the native mechanism of a platform that has none. It
// never authenticates anyone: Windows refuses unless the server is paired with
// IPFS Desktop, whose pairing lands separately.
type refusingFactor struct {
	resolver string
	why      string
}

func (f refusingFactor) name() string { return f.resolver }

func (f refusingFactor) check(context.Context) error {
	return fmt.Errorf("%w: %s", ErrUnavailable, f.why)
}

func (f refusingFactor) authenticate(context.Context, string) (bool, error) {
	return false, fmt.Errorf("%w: %s", ErrUnavailable, f.why)
}
