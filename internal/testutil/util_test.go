package testutil

type (
	mockCloser struct {
		close func() error
	}
)

func (x *mockCloser) Close() error { return x.close() }
