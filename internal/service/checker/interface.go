package checker

import "context"

type CheckerService interface {
	Start(ctx context.Context)
	Stop()
	Pause()
	Resume()
	IsPaused() bool
}
