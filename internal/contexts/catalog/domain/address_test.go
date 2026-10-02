package domain

import "testing"

func TestParseRepositoryURLAcceptsGitHubRepositoryURLs(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/fabricahq/code-rules-test-library",
		"https://github.com/fabricahq/code-rules-test-library/",
		"https://github.com/fabricahq/code-rules-test-library.git",
	} {
		owner, name, err := ParseRepositoryURL(raw)
		if err != nil || owner != "fabricahq" || name != "code-rules-test-library" {
			t.Errorf("%s: got %q, %q, %v", raw, owner, name, err)
		}
	}
}

func TestParseRepositoryURLRejectsOtherURLs(t *testing.T) {
	for _, raw := range []string{
		"fabricahq/code-rules-test-library",
		"http://github.com/fabricahq/code-rules-test-library",
		"https://gitlab.com/fabricahq/code-rules-test-library",
		"https://github.com/fabricahq",
		"https://github.com/fabricahq/code-rules-test-library/tree/main",
		"https://github.com/fabricahq/code-rules-test-library?tab=readme",
	} {
		if _, _, err := ParseRepositoryURL(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestStorableRefusesNULAndInvalidUTF8(t *testing.T) {
	for text, want := range map[string]bool{
		"": true, "fabricahq/public-rules": true, "règles/中文": true,
		"a\x00b": false, "\x00": false, "a\xffb": false, "\xc3\x28": false,
	} {
		if got := Storable(text); got != want {
			t.Errorf("Storable(%q) = %v, want %v", text, got, want)
		}
	}
}
