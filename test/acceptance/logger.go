package acceptance

import "testing"

type logInterceptor struct {
	t        *testing.T
	suppress bool
}

func (l logInterceptor) Write(p []byte) (n int, err error) {
	if !l.suppress {
		l.t.Log((string)(p))
	}
	return len(p), nil
}
