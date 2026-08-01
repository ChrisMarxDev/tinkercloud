package main

import "strings"

const testResendCredentialValue = "re_" + "test_correct_key"

func testHMACCredentialValue() string {
	return strings.Repeat("0", 32)
}

func credentialTestFixture(resend string) string {
	return strings.Join([]string{
		"RESEND_API_KEY=" + resend,
		"TINKERCLOUD_HMAC_KEY=" + testHMACCredentialValue(),
		"",
	}, "\n")
}
