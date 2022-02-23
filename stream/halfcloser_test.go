package stream

import (
	"testing"
	"time"
)

func TestUnwrapClosePolicy(t *testing.T) {
	for _, tc := range [...]struct {
		Name   string
		Input  ClosePolicy
		Output ClosePolicy
	}{
		{
			Name:   `nil`,
			Output: WaitRemote{},
		},
		{
			Name:   `wait remote`,
			Input:  WaitRemote{},
			Output: WaitRemote{},
		},
		{
			Name:   `wait remote timeout`,
			Input:  WaitRemoteTimeout(time.Minute * 5),
			Output: WaitRemoteTimeout(time.Minute * 5),
		},
		{
			Name:   `embedded nil interface`,
			Input:  struct{ ClosePolicy }{},
			Output: WaitRemote{},
		},
		{
			Name:   `embedded value`,
			Input:  struct{ WaitRemoteTimeout }{WaitRemoteTimeout(time.Second * 30)},
			Output: WaitRemoteTimeout(time.Second * 30),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			if output := UnwrapClosePolicy(tc.Input); output != tc.Output {
				t.Error(output)
			}
		})
	}
}
