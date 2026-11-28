package capture

import "testing"

func TestDecodeLSOFName(t *testing.T) {
	for input, want := range map[string]string{
		`\xe4\xbc\x81\xe4\xb8\x9a\xe5\xbe\xae\xe4\xbf\xa1`: "企业微信",
		"Google Chrome Helper":                             "Google Chrome Helper",
		`App \\xe4\xZZ`:                                    `App \xe4\xZZ`,
		`\xe4\xbc`:                                         "�",
	} {
		if got := decodeLSOFName(input); got != want {
			t.Errorf("decodeLSOFName(%q) = %q, want %q", input, got, want)
		}
	}
}
