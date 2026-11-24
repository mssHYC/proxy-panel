package handler

import (
	"strings"
	"testing"
)

func TestRealityCatalogKeepsNearHTTPSHostsAndRegisteredCountries(t *testing.T) {
	const catalog = `<metalink><files><file><resources>
	<url location="CA" preference="80">https://second.example/repo</url>
	<url location="US" preference="90">https://first.example/repo</url>
	<url location="GB" preference="60">https://FIRST.example:443/other</url>
	<url preference="99">http://plain.example/repo</url>
	<url preference="99">https://192.0.2.1/repo</url>
	<url preference="99">https://alt.example:444/repo</url>
	<url preference="99">https://user@credential.example/repo</url>
	</resources></file></files></metalink>`
	got, err := readRealityMirrors(strings.NewReader(catalog))
	if err != nil || len(got) != 2 || got[0] != (realityMirror{"first.example", "US"}) || got[1] != (realityMirror{"second.example", "CA"}) {
		t.Fatalf("mirrors = %v, err = %v", got, err)
	}
	for _, invalid := range []string{"<broken>", "<metalink/>", catalog + strings.Repeat(" ", 256*1024)} {
		if _, err := readRealityMirrors(strings.NewReader(invalid)); err == nil {
			t.Fatal("accepted unusable directory")
		}
	}
}
