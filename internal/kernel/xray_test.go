package kernel

import "testing"

func TestGetSettingInt(t *testing.T) {
	cases := []struct {
		name     string
		m        map[string]interface{}
		key      string
		def      int
		expected int
	}{
		{"nil map returns default", nil, "x", 7, 7},
		{"missing key returns default", map[string]interface{}{}, "x", 7, 7},
		{"int value", map[string]interface{}{"x": 5}, "x", 7, 5},
		{"float64 from json", map[string]interface{}{"x": 5.0}, "x", 7, 5},
		{"int64 value", map[string]interface{}{"x": int64(5)}, "x", 7, 5},
		{"numeric string", map[string]interface{}{"x": "12"}, "x", 7, 12},
		{"invalid string falls back", map[string]interface{}{"x": "abc"}, "x", 7, 7},
		{"empty string falls back", map[string]interface{}{"x": ""}, "x", 7, 7},
		{"wrong type falls back", map[string]interface{}{"x": []int{1}}, "x", 7, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := getSettingInt(c.m, c.key, c.def); got != c.expected {
				t.Errorf("%s: want %d, got %d", c.name, c.expected, got)
			}
		})
	}
}
