// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
)

var warningURLPattern = regexp.MustCompile(`https?://\S+`)

// sanitizeURL drops userinfo and strips sensitive query keys from a URL so it
// can be emitted in a fact. It fails closed: a value that cannot be taken
// apart is replaced with the empty string, never passed through.
func sanitizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		// Fail closed: nothing can prove a string credential-free when it
		// cannot be taken apart.
		return ""
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		// Not hierarchical, so User can be nil with a credential in plain
		// sight (`svc:SECRET@host/x` keeps it in Opaque). Ask net/url about
		// the authority spelling and drop the value when it carries userinfo.
		if carriesUserinfo(trimmed) {
			return ""
		}
		return trimmed
	}
	parsed.User = nil
	query := parsed.Query()
	for key := range query {
		if isSensitiveQueryKey(key) {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// sanitizeText redacts credential-shaped URLs inside free-text messages.
func sanitizeText(input string) string {
	return warningURLPattern.ReplaceAllStringFunc(input, sanitizeURL)
}

func isSensitiveQueryKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	sensitiveKeys := []string{
		"access_token",
		"api_key",
		"apikey",
		"auth",
		"authorization",
		"jwt",
		"key",
		"password",
		"passwd",
		"secret",
		"sig",
		"signature",
		"token",
		"x-amz-credential",
		"x-amz-security-token",
		"x-amz-signature",
	}
	return slices.Contains(sensitiveKeys, normalized)
}

// carriesUserinfo reports whether value embeds a credential in userinfo
// position under any reading net/url accepts: the direct parse, or the
// authority re-parse performed when an opaque body is shaped like an
// authority. An unparseable value reports true — nothing can prove a string
// credential-free when it cannot be taken apart.
func carriesUserinfo(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return true
	}
	if !opaqueHasAuthority(parsed.Opaque) {
		return parsed.User != nil
	}
	authority, err := url.Parse("//" + value)
	if err != nil {
		return true
	}
	return authority.User != nil
}

// opaqueHasAuthority reports whether an opaque URL body (everything after
// "scheme:" when no "//" follows it) begins with something shaped like an
// authority: an "@" ahead of the first "/".
func opaqueHasAuthority(opaque string) bool {
	if opaque == "" {
		return false
	}
	authority := opaque
	if slash := strings.IndexByte(opaque, '/'); slash >= 0 {
		authority = opaque[:slash]
	}
	return strings.IndexByte(authority, '@') >= 0
}
