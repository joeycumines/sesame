package testutil

func CallOn[T any](v T, f func(v T)) T {
	f(v)
	return v
}
