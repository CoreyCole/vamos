package build

import "testing"

func TestParseForce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     string
		want      ForceSet
		wantError bool
	}{
		{name: "empty", value: "", want: ForceSet{}},
		{name: "all", value: "all", want: ForceSet{ForceAll: true}},
		{
			name:  "targets",
			value: "templ,tailwind",
			want:  ForceSet{ForceTempl: true, ForceTailwind: true},
		},
		{
			name:  "proto and datastar assets",
			value: "proto,datastar-assets",
			want:  ForceSet{ForceProto: true, ForceDatastarAssets: true},
		},
		{name: "unknown", value: "bad", wantError: true},
		{name: "trailing comma", value: "templ,", wantError: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseForce(test.value)
			if test.wantError {
				if err == nil {
					t.Fatalf("ParseForce(%q) succeeded, want error", test.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseForce(%q): %v", test.value, err)
			}
			if len(got) != len(test.want) {
				t.Fatalf(
					"ParseForce(%q) len = %d, want %d",
					test.value,
					len(got),
					len(test.want),
				)
			}
			for target := range test.want {
				if !got[target] {
					t.Fatalf("ParseForce(%q)[%q] = false, want true", test.value, target)
				}
			}
		})
	}
}

func TestForceSetHas(t *testing.T) {
	t.Parallel()

	if ForceSet(nil).Has(ForceTempl) {
		t.Fatal("nil ForceSet.Has(templ) = true, want false")
	}
	if !(ForceSet{ForceAll: true}).Has(ForceTSWorker) {
		t.Fatal("ForceSet{all}.Has(ts-worker) = false, want true")
	}
	if !(ForceSet{ForceTempl: true}).Has(ForceTempl) {
		t.Fatal("ForceSet{templ}.Has(templ) = false, want true")
	}
	if (ForceSet{ForceTempl: true}).Has(ForceGo) {
		t.Fatal("ForceSet{templ}.Has(go) = true, want false")
	}
}
