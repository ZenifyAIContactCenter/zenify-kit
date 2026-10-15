// Package e2e serves POST /e2e/account: one staging account per developer
// and role, with a password derived from the broker's HMAC key.
package e2e

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// passwordSuffix guarantees the lower, upper, digit and special classes of
// be's PASSWORD_PATTERN whatever the HMAC prefix holds.
const passwordSuffix = "Aa1!"

// Password is P(period) for login and role (FR-5.1): HMAC-SHA256 of
// "login:role:period", base64url without '-' and '_', first 20 characters,
// then passwordSuffix.
func Password(key []byte, login, role string, period int64) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(login + ":" + role + ":" + strconv.FormatInt(period, 10)))
	s := strings.NewReplacer("-", "", "_", "").Replace(base64.RawURLEncoding.EncodeToString(m.Sum(nil)))
	return s[:min(20, len(s))] + passwordSuffix
}

// Period is the 90-day rotation period of now: floor(unix_days / 90).
func Period(now time.Time) int64 { return now.Unix() / 86400 / 90 }
