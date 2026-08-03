package repository

import "testing"

func TestNormalizeTelegramChannel(t *testing.T) {
	cases := map[string]string{
		"https://t.me/s/Test_Channel/": "test_channel",
		"@Test_Channel":                "test_channel",
		"https://t.me/channel?x=1":     "",
		"bad/channel":                  "",
	}
	for input, want := range cases {
		if got := NormalizeTelegramChannel(input); got != want {
			t.Fatalf("NormalizeTelegramChannel(%q) = %q; want %q", input, got, want)
		}
	}
}

