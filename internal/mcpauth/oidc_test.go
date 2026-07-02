package mcpauth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/briancolinger/persistor/internal/mcpauth"
)

const (
	testIssuer   = "https://test.stytch.example"
	testAudience = "persistor-mcp"
)

func staticKeyFunc(pub *rsa.PublicKey) jwt.Keyfunc {
	return func(*jwt.Token) (any, error) { return pub, nil }
}

func signRS256(t *testing.T, key *rsa.PrivateKey, claims *jwt.RegisteredClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func validClaims() jwt.RegisteredClaims {
	return jwt.RegisteredClaims{
		Issuer:    testIssuer,
		Subject:   "user-abc",
		Audience:  jwt.ClaimStrings{testAudience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
}

func TestOIDCAuth_Verify(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey other: %v", err)
	}
	ctx := context.Background()
	a := mcpauth.NewOIDCAuth(staticKeyFunc(&key.PublicKey), testIssuer, testAudience)

	// Valid token -> tenant = uuidv5(iss|sub), expiry carried through.
	valid := validClaims()
	ti, err := a.Verify(ctx, signRS256(t, key, &valid), nil)
	if err != nil {
		t.Fatalf("valid: %v", err)
	}
	if want := mcpauth.TenantForSubject(testIssuer, "user-abc"); ti.UserID != want {
		t.Fatalf("UserID = %q, want %q", ti.UserID, want)
	}
	if !ti.Expiration.After(time.Now()) {
		t.Fatalf("expiration %v not in the future", ti.Expiration)
	}

	expired := validClaims()
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	wrongIss := validClaims()
	wrongIss.Issuer = "https://evil.example"
	wrongAud := validClaims()
	wrongAud.Audience = jwt.ClaimStrings{"someone-else"}
	noSub := validClaims()
	noSub.Subject = ""
	// A subject containing the tenant-derivation separator "|" is rejected so the
	// issuer|subject join can't be made ambiguous (a cross-tenant collision).
	sepSub := validClaims()
	sepSub.Subject = "user|admin"
	badSig := validClaims()
	// alg confusion: an HS256 token (signed with any secret) must be rejected by
	// WithValidMethods before the key function is ever consulted.
	hsClaims := validClaims()
	hs256, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &hsClaims).SignedString([]byte("pubkey-as-secret"))
	if err != nil {
		t.Fatalf("sign hs256: %v", err)
	}

	cases := []struct {
		name  string
		token string
	}{
		{"expired", signRS256(t, key, &expired)},
		{"wrong issuer", signRS256(t, key, &wrongIss)},
		{"wrong audience", signRS256(t, key, &wrongAud)},
		{"bad signature", signRS256(t, other, &badSig)},
		{"no subject", signRS256(t, key, &noSub)},
		{"subject with separator", signRS256(t, key, &sepSub)},
		{"alg confusion", hs256},
		{"garbage", "not.a.jwt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := a.Verify(ctx, tc.token, nil); !errors.Is(err, auth.ErrInvalidToken) {
				t.Fatalf("want auth.ErrInvalidToken, got %v", err)
			}
		})
	}
}

// TestOIDCAuth_TokenTypeSeparation covers the access-token assertion: ID-token
// marker claims (at_hash, nonce) and non-access typ headers are rejected even
// when the signature, issuer, and audience all validate; access-token typ
// headers (JWT / at+jwt) pass.
func TestOIDCAuth_TokenTypeSeparation(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	ctx := context.Background()
	a := mcpauth.NewOIDCAuth(staticKeyFunc(&key.PublicKey), testIssuer, testAudience)

	sign := func(extra map[string]any, typ string) string {
		claims := jwt.MapClaims{
			"iss": testIssuer,
			"sub": "user-abc",
			"aud": testAudience,
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		}
		for k, v := range extra {
			claims[k] = v
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		if typ != "" {
			tok.Header["typ"] = typ
		}
		s, err := tok.SignedString(key)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return s
	}

	rejected := []struct {
		name  string
		token string
	}{
		{"id token via at_hash", sign(map[string]any{"at_hash": "abc123"}, "JWT")},
		{"id token via nonce", sign(map[string]any{"nonce": "n-0S6_WzA2Mj"}, "JWT")},
		{"foreign typ header", sign(nil, "secevent+jwt")},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := a.Verify(ctx, tc.token, nil); !errors.Is(err, auth.ErrInvalidToken) {
				t.Fatalf("want auth.ErrInvalidToken, got %v", err)
			}
		})
	}

	accepted := []struct {
		name  string
		token string
	}{
		{"generic JWT typ", sign(nil, "JWT")},
		{"rfc9068 at+jwt", sign(nil, "at+jwt")},
		{"rfc9068 full mediatype", sign(nil, "application/at+jwt")},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := a.Verify(ctx, tc.token, nil); err != nil {
				t.Fatalf("access token rejected: %v", err)
			}
		})
	}
}

// TestOIDCAuth_AllowedSubjects covers the provisioning lock: with a non-empty
// allowlist only listed subjects verify; an empty allowlist stays open.
func TestOIDCAuth_AllowedSubjects(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	ctx := context.Background()
	sign := func(sub string) string {
		c := validClaims()
		c.Subject = sub
		return signRS256(t, key, &c)
	}

	// Open (nil / empty allowlist): any valid subject passes.
	open := mcpauth.NewOIDCAuth(staticKeyFunc(&key.PublicKey), testIssuer, testAudience).
		WithAllowedSubjects(nil)
	if _, err := open.Verify(ctx, sign("anyone"), nil); err != nil {
		t.Errorf("open allowlist rejected a valid token: %v", err)
	}

	// Closed: only the listed subject passes; others are rejected as invalid.
	closed := mcpauth.NewOIDCAuth(staticKeyFunc(&key.PublicKey), testIssuer, testAudience).
		WithAllowedSubjects([]string{" user-live-demo ", ""}) // trimmed + blanks dropped
	if _, err := closed.Verify(ctx, sign("user-live-demo"), nil); err != nil {
		t.Errorf("allowlisted subject rejected: %v", err)
	}
	if _, err := closed.Verify(ctx, sign("user-live-stranger"), nil); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("unlisted subject: want auth.ErrInvalidToken, got %v", err)
	}
}

// fakeTenantResolver stands in for the identities-backed tenant resolver.
type fakeTenantResolver struct {
	tenant     string
	role       string // defaults to "owner" when empty
	err        error
	gotDefault string
}

func (f *fakeTenantResolver) ResolveOrProvision(_ context.Context, _, _, defaultTenant string) (tenantID, role string, err error) {
	f.gotDefault = defaultTenant
	if f.err != nil {
		return "", "", f.err
	}
	role = f.role
	if role == "" {
		role = "owner"
	}
	return f.tenant, role, nil
}

// TestIsReadOnly covers the role gate read at the tool boundary.
func TestIsReadOnly(t *testing.T) {
	tests := []struct {
		name string
		ti   *auth.TokenInfo
		want bool
	}{
		{name: "nil token", ti: nil, want: false},
		{name: "no role (static key / owner)", ti: &auth.TokenInfo{}, want: false},
		{name: "owner", ti: &auth.TokenInfo{Extra: map[string]any{"role": "owner"}}, want: false},
		{name: "member", ti: &auth.TokenInfo{Extra: map[string]any{"role": "member"}}, want: false},
		{name: "readonly", ti: &auth.TokenInfo{Extra: map[string]any{"role": "readonly"}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mcpauth.IsReadOnly(tt.ti); got != tt.want {
				t.Fatalf("IsReadOnly = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOIDCAuth_TenantResolver(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	ctx := context.Background()

	// A resolver-supplied tenant overrides the claim-derived default, and the
	// default handed to the resolver is exactly uuidv5(iss|sub).
	assigned := uuid.NewString()
	fr := &fakeTenantResolver{tenant: assigned}
	a := mcpauth.NewOIDCAuth(staticKeyFunc(&key.PublicKey), testIssuer, testAudience).WithTenantResolver(fr)
	valid := validClaims()
	ti, err := a.Verify(ctx, signRS256(t, key, &valid), nil)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ti.UserID != assigned {
		t.Fatalf("UserID = %q, want resolver tenant %q", ti.UserID, assigned)
	}
	if want := mcpauth.TenantForSubject(testIssuer, "user-abc"); fr.gotDefault != want {
		t.Fatalf("resolver default = %q, want uuidv5 %q", fr.gotDefault, want)
	}
	if mcpauth.IsReadOnly(ti) {
		t.Fatal("owner identity should not be read-only")
	}

	// A readonly identity's role must travel to the tool boundary via Extra.
	ro := &fakeTenantResolver{tenant: uuid.NewString(), role: "readonly"}
	aRO := mcpauth.NewOIDCAuth(staticKeyFunc(&key.PublicKey), testIssuer, testAudience).WithTenantResolver(ro)
	tiRO, err := aRO.Verify(ctx, signRS256(t, key, &valid), nil)
	if err != nil {
		t.Fatalf("verify readonly: %v", err)
	}
	if !mcpauth.IsReadOnly(tiRO) {
		t.Fatal("readonly identity not flagged read-only at the boundary")
	}

	// A resolver (storage) error is a server error, NOT a bad token: it must not
	// unwrap to auth.ErrInvalidToken (which would wrongly become a 401).
	fe := &fakeTenantResolver{err: errors.New("db down")}
	a2 := mcpauth.NewOIDCAuth(staticKeyFunc(&key.PublicKey), testIssuer, testAudience).WithTenantResolver(fe)
	if _, err := a2.Verify(ctx, signRS256(t, key, &valid), nil); err == nil {
		t.Fatal("want error on resolver failure")
	} else if errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("resolver failure must not be ErrInvalidToken, got %v", err)
	}
}

func TestTenantForSubject_Deterministic(t *testing.T) {
	a := mcpauth.TenantForSubject(testIssuer, "user-1")
	b := mcpauth.TenantForSubject(testIssuer, "user-1")
	c := mcpauth.TenantForSubject(testIssuer, "user-2")
	if a != b {
		t.Fatalf("not deterministic: %q vs %q", a, b)
	}
	if a == c {
		t.Fatal("different subjects collided to the same tenant")
	}
	if _, err := uuid.Parse(a); err != nil {
		t.Fatalf("tenant %q is not a UUID: %v", a, err)
	}
}
