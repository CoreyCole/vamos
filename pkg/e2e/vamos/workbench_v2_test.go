package vamos

import "testing"

func TestWorkbenchOrderValueAcceptsJSONNumberRepresentations(t *testing.T) {
	for _, test := range []struct {
		value any
		want  int
	}{
		{value: 3, want: 3},
		{value: int64(4), want: 4},
		{value: float64(5), want: 5},
		{value: "6", want: -1},
	} {
		if got := workbenchOrderValue(test.value); got != test.want {
			t.Fatalf(
				"workbenchOrderValue(%T(%v)) = %d, want %d",
				test.value,
				test.value,
				got,
				test.want,
			)
		}
	}
}
