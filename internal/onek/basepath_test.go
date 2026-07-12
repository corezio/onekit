package onek

import "testing"

func TestInferBasePath(t *testing.T) {
	cases := []struct {
		relDir string
		want   string
	}{
		{".", "/"},
		{"", "/"},
		{"hub/business/v1", "/hub/business/v1"},
		{"hr/time_entry/v1", "/hr/time-entry/v1"},
		{"hr/one_on_one/v1", "/hr/one-on-one/v1"},
		{"pos/menu_item/v1", "/pos/menu-item/v1"},
		{"hr/employees/payroll/runs/v1", "/hr/employees/payroll/runs/v1"},
	}
	for _, c := range cases {
		if got := inferBasePath(c.relDir); got != c.want {
			t.Errorf("inferBasePath(%q) = %q, want %q", c.relDir, got, c.want)
		}
	}
}
