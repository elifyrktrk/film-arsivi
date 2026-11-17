package i18n

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestSharedStringsMatchTS checks every shared (non-Web) string still appears verbatim
// in packages/i18n/src/index.ts, so the web and mobile apps say the same thing.
func TestSharedStringsMatchTS(t *testing.T) {
	src, err := os.ReadFile("../../../../packages/i18n/src/index.ts")
	if err != nil {
		t.Skip("packages/i18n not found next to apps/web:", err)
	}
	ts := string(src)
	var walk func(path string, v reflect.Value)
	walk = func(path string, v reflect.Value) {
		switch v.Kind() {
		case reflect.String:
			if !strings.Contains(ts, `"`+v.String()+`"`) {
				t.Errorf("%s = %q is not in packages/i18n", path, v.String())
			}
		case reflect.Map:
			for _, k := range v.MapKeys() {
				if k.String() == "invalid_color" {
					continue // web-only
				}
				walk(path+"."+k.String(), v.MapIndex(k))
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).Name == "Web" {
					continue // web-only
				}
				walk(path+"."+v.Type().Field(i).Name, v.Field(i))
			}
		}
	}
	walk("TR", reflect.ValueOf(TR))
	for _, f := range []string{"için sonuç yok", "sezon`", "bölüm`", "içerik`", `"Boş"`} {
		if !strings.Contains(ts, f) {
			t.Errorf("formatter text %q changed in packages/i18n", f)
		}
	}
}
