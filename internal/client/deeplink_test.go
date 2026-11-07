package client

import (
	"net/url"
	"strings"
	"testing"
)

func TestOpenLinkExactVersionAndSource(t *testing.T) {
	link := "skillhub://open?" + url.Values{"hub": {"https://skills.codectn.com"}, "skill": {"team/review"}, "version": {"1.2.3-beta.1"}}.Encode()
	r, err := ParseOpenLink(link)
	if err != nil || r.Namespace != "team" || r.Name != "review" || r.Version != "1.2.3-beta.1" {
		t.Fatal(r, err)
	}
	if !strings.Contains(r.WebURL(), "version=1.2.3-beta.1") || strings.Contains(r.WebURL(), "token") {
		t.Fatal("unsafe web link")
	}
	for _, bad := range []string{strings.Replace(link, "1.2.3-beta.1", "1", 1), strings.Replace(link, "1.2.3-beta.1", "1.2", 1), link + "&token=secret", link + "&hub=https://other.example", link + "&install=true", link + "#fragment", strings.Replace(link, "skillhub://open", "skillhub://other", 1), strings.Replace(link, "1.2.3-beta.1", "latest", 1), "skillhub://open?hub=https%3A%2F%2Fu%3Ap%40example.com&skill=team%2Freview&version=1.0.0", "skillhub://open?hub=file%3A%2F%2Ftmp&skill=team%2Freview&version=1.0.0", strings.Repeat("x", 2049)} {
		if _, err := ParseOpenLink(bad); err == nil {
			t.Fatal("accepted malformed link")
		}
	}
}
