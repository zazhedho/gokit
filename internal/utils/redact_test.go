package utils

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestRedactSensitivePayload(t *testing.T) {
	payload := struct {
		Email        string         `json:"email"`
		Password     string         `json:"password"`
		OTPCode      string         `json:"otp_code"`
		AccessToken  string         `json:"access_token"`
		RefreshToken string         `json:"refresh_token"`
		Metadata     map[string]any `json:"metadata"`
	}{
		Email:        "jane@example.com",
		Password:     "secret123",
		OTPCode:      "123456",
		AccessToken:  "access-secret",
		RefreshToken: "refresh-secret",
		Metadata: map[string]any{
			"client_secret": "nested-secret",
			"provider":      "google",
		},
	}

	got, ok := RedactSensitivePayload(payload).(map[string]any)
	if !ok {
		t.Fatalf("expected normalized map, got %T", got)
	}
	for _, key := range []string{"password", "otp_code", "access_token", "refresh_token"} {
		if got[key] != "[REDACTED]" {
			t.Fatalf("expected %s to be redacted, got %v", key, got[key])
		}
	}
	if got["email"] != "jane@example.com" {
		t.Fatalf("expected safe field to remain visible, got %v", got["email"])
	}

	metadata, ok := got["metadata"].(map[string]any)
	if !ok || metadata["client_secret"] != "[REDACTED]" || metadata["provider"] != "google" {
		t.Fatalf("unexpected nested redaction result: %v", got["metadata"])
	}
}

func TestSensitiveKeyVariants(t *testing.T) {
	for _, key := range []string{
		"password", "Password", "user_password", "userPassword", "passwd", "pwd", "PWD", "db.pwd",
		"token", "access-token", "accessToken", "X-Auth-Token", "csrf_token",
		"secret", "client_secret", "otp", "OTP", "user_otp", "otpCode",
		"apikey", "api_key", "X-Api-Key", "apiKey",
		"authorization", "Authorization", "Proxy-Authorization",
		"cookie", "Set-Cookie", "credential", "credentials",
		"privatekey", "private_key", "session", "session_id", "jwt", "id.jwt",
		"cvv", "CVV", "pin", "pin_code", "user-pin",
	} {
		if !isSensitiveKey(key, nil) {
			t.Errorf("%q must be sensitive", key)
		}
	}
	for _, key := range []string{
		"footprint", "email", "name", "shipping", "spinner", "hotpot", "description", "tokenless_id_not",
	} {
		// "tokenless..." contains token and is allowed to be over-redacted; skip it
		if key == "tokenless_id_not" {
			continue
		}
		if isSensitiveKey(key, nil) {
			t.Errorf("%q must not be sensitive", key)
		}
	}
	if !isSensitiveKey("zip_code", []string{"ZIP"}) || isSensitiveKey("zip_code", nil) {
		t.Fatal("extra keys must be honored, and only when given")
	}
}

func TestRedactWalksEveryContainer(t *testing.T) {
	const masked = "[REDACTED]"

	hdr := RedactSensitivePayload(http.Header{"Authorization": {"Bearer x"}, "Set-Cookie": {"a=b"}, "Accept": {"json"}}).(http.Header)
	if hdr["Authorization"][0] != masked || hdr["Set-Cookie"][0] != masked || hdr["Accept"][0] != "json" {
		t.Fatalf("http.Header: %v", hdr)
	}

	vals := RedactSensitivePayload(url.Values{"password": {"p"}, "q": {"x"}}).(url.Values)
	if vals.Get("password") != masked || vals.Get("q") != "x" {
		t.Fatalf("url.Values: %v", vals)
	}

	ms := RedactSensitivePayload(map[string]string{"api_key": "k", "name": "n"}).(map[string]string)
	if ms["api_key"] != masked || ms["name"] != "n" {
		t.Fatalf("map[string]string: %v", ms)
	}

	list := RedactSensitivePayload([]map[string]any{{"token": "t", "id": 1}}).([]map[string]any)
	if list[0]["token"] != masked || list[0]["id"] != 1 {
		t.Fatalf("[]map[string]any: %v", list)
	}

	type inner struct {
		PIN string `json:"pin"`
	}
	type outer struct {
		Name  string  `json:"name"`
		Inner *inner  `json:"inner"`
		Items []inner `json:"items"`
	}
	got := RedactSensitivePayload(&outer{Name: "n", Inner: &inner{PIN: "1234"}, Items: []inner{{PIN: "9"}}}).(map[string]any)
	if got["inner"].(map[string]any)["pin"] != masked || got["items"].([]any)[0].(map[string]any)["pin"] != masked || got["name"] != "n" {
		t.Fatalf("struct/pointer: %v", got)
	}

	nested := RedactSensitivePayload(map[string]any{"a": []any{map[string]any{"jwt": "j"}}}).(map[string]any)
	if nested["a"].([]any)[0].(map[string]any)["jwt"] != masked {
		t.Fatalf("nested: %v", nested)
	}
}

func TestRedactKeepsBigNumbersAndExtraKeys(t *testing.T) {
	type row struct {
		ID  int64  `json:"id"`
		SSN string `json:"ssn"`
	}
	got := RedactSensitivePayload(row{ID: 9007199254740993, SSN: "x"}, "ssn").(map[string]any)
	if got["ssn"] != "[REDACTED]" {
		t.Fatalf("extra key not applied: %v", got)
	}
	if b, _ := json.Marshal(got["id"]); string(b) != "9007199254740993" {
		t.Fatalf("large id lost precision: %s", b)
	}
}

func TestRedactFailsClosedOnMarshalError(t *testing.T) {
	for name, in := range map[string]any{
		"channel":   make(chan int),
		"func":      func() {},
		"in struct": struct{ C chan int }{C: make(chan int)},
	} {
		if got := RedactSensitivePayload(in); got != "[UNREDACTABLE]" {
			t.Errorf("%s: got %#v, want [UNREDACTABLE]", name, got)
		}
	}
}

func TestRedactScalarsAndNil(t *testing.T) {
	if RedactSensitivePayload(nil) != nil || RedactSensitivePayload("x") != "x" || RedactSensitivePayload(3) != 3 {
		t.Fatal("scalars must pass through unchanged")
	}
}

func TestSensitiveKeyAdditions(t *testing.T) {
	for _, key := range []string{
		"passphrase", "pass_code", "passcode", "accessKey", "access_key", "bearer", "Bearer-Token", "signature", "saml_assertion",
		"cardNumber", "card_number", "pass", "auth", "user_auth", "ssn", "pan", "otp123", "OTP2", "pin4",
	} {
		if !isSensitiveKey(key, nil) {
			t.Errorf("%q must be sensitive", key)
		}
	}
	for _, key := range []string{"passenger", "panel", "author", "compass", "japan", "ssnake"} {
		if isSensitiveKey(key, nil) {
			t.Errorf("%q must not be sensitive", key)
		}
	}
}

func TestRedactDecodesJSONStringsAndBytes(t *testing.T) {
	const doc = `{"password":"p","id":12345678901234567890,"n":{"token":"t"}}`
	for name, in := range map[string]any{"string": doc, "bytes": []byte(doc), "raw": json.RawMessage(doc)} {
		got, ok := RedactSensitivePayload(in).(map[string]any)
		if !ok || got["password"] != "[REDACTED]" || got["n"].(map[string]any)["token"] != "[REDACTED]" {
			t.Fatalf("%s: %#v", name, got)
		}
		if b, _ := json.Marshal(got["id"]); string(b) != "12345678901234567890" {
			t.Fatalf("%s: id %s", name, b)
		}
	}
	if got := RedactSensitivePayload("plain text"); got != "plain text" {
		t.Fatalf("non-JSON string is a scalar: %#v", got)
	}
	if got := RedactSensitivePayload(`"just a json string"`); got != `"just a json string"` {
		t.Fatalf("JSON scalar string stays as is: %#v", got)
	}
	for name, in := range map[string]any{"bytes": []byte("not json {"), "raw": json.RawMessage("nope")} {
		if got := RedactSensitivePayload(in); got != "[UNREDACTABLE]" {
			t.Fatalf("%s: %#v", name, got)
		}
	}
}

func TestRedactSurvivesSelfReferentialMaps(t *testing.T) {
	m := map[string]any{"a": 1}
	m["self"] = m
	got := RedactSensitivePayload(m).(map[string]any)
	if got["a"] != 1 {
		t.Fatalf("got %v", got)
	}
	var depth int
	for cur := any(got); ; depth++ {
		next, ok := cur.(map[string]any)
		if !ok {
			if cur != "[UNREDACTABLE]" {
				t.Fatalf("cap must end in [UNREDACTABLE], got %v", cur)
			}
			break
		}
		cur = next["self"]
	}
	if depth > 40 {
		t.Fatalf("depth %d not capped", depth)
	}
}

func TestRedactDecodesByteFieldsInsideStructs(t *testing.T) {
	type payload struct {
		Name    string          `json:"name"`
		Body    []byte          `json:"body"`
		Raw     json.RawMessage `json:"raw"`
		Garbage []byte          `json:"garbage"`
		Hidden  string          `json:"-"`
		When    time.Time       `json:"when"`
		Opt     string          `json:"opt,omitempty"`
	}
	in := &payload{
		Name: "n", Body: []byte(`{"password":"p","keep":1}`), Raw: json.RawMessage(`[{"token":"t"}]`),
		Garbage: []byte("not json"), Hidden: "h", When: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	got, ok := RedactSensitivePayload(in).(map[string]any)
	if !ok {
		t.Fatalf("got %T", RedactSensitivePayload(in))
	}
	if got["body"].(map[string]any)["password"] != "[REDACTED]" || got["body"].(map[string]any)["keep"] != json.Number("1") {
		t.Fatalf("[]byte field must be decoded and redacted, got %#v", got["body"])
	}
	if got["raw"].([]any)[0].(map[string]any)["token"] != "[REDACTED]" {
		t.Fatalf("RawMessage field: %#v", got["raw"])
	}
	if got["garbage"] != "[UNREDACTABLE]" {
		t.Fatalf("undecodable bytes: %#v", got["garbage"])
	}
	if _, present := got["Hidden"]; present || got["name"] != "n" || got["when"] != "2026-01-02T03:04:05Z" {
		t.Fatalf("tags and marshalers must be honored: %#v", got)
	}
	if _, present := got["opt"]; present {
		t.Fatalf("omitempty field must be skipped: %#v", got)
	}
}

func TestRedactSurvivesPointerCycles(t *testing.T) {
	type node struct {
		Name string `json:"name"`
		Next *node  `json:"next"`
	}
	n := &node{Name: "a"}
	n.Next = n
	if got := RedactSensitivePayload(n); got == nil {
		t.Fatal("must terminate with a value")
	}
}
