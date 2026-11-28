package identityhttp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"strings"
	"sync"
	"time"
)

const (
	privacyPublicRequestsPath       = "/api/v1/privacy/public-requests"
	privacyPublicRequestsLookupPath = "/api/v1/privacy/public-requests/lookup"
	publicPrivacyCreateGlobalLimit  = 1200
	publicPrivacyLookupGlobalLimit  = 3000
	maxPublicPrivacyRateLimitKeys   = publicPrivacyCreateGlobalLimit + publicPrivacyLookupGlobalLimit + 2
)

type publicPrivacyRateLimitBucket struct {
	requests []time.Time
	window   time.Duration
	limit    int
}

type publicPrivacyRateLimiter struct {
	mu          sync.Mutex
	key         [32]byte
	buckets     map[[32]byte]publicPrivacyRateLimitBucket
	lastCleanup time.Time
	lastNow     time.Time
}

func newPublicPrivacyRateLimiter() *publicPrivacyRateLimiter {
	limiter := &publicPrivacyRateLimiter{buckets: make(map[[32]byte]publicPrivacyRateLimitBucket)}
	rand.Read(limiter.key[:])
	return limiter
}

func (l *publicPrivacyRateLimiter) allow(path, identifier string, now time.Time) (bool, time.Duration) {
	identifierLimit, identifierWindow := 5, time.Hour
	globalLimit, globalWindow := publicPrivacyCreateGlobalLimit, time.Hour
	canonicalIdentifier := strings.ToLower(strings.TrimSpace(identifier))
	if path == privacyPublicRequestsLookupPath {
		identifierLimit, identifierWindow = 20, 15*time.Minute
		globalLimit, globalWindow = publicPrivacyLookupGlobalLimit, 15*time.Minute
		canonicalIdentifier = strings.ToUpper(strings.TrimSpace(identifier))
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Before(l.lastNow) {
		now = l.lastNow
	}
	l.lastNow = now
	if l.lastCleanup.IsZero() || now.Sub(l.lastCleanup) >= time.Minute {
		l.pruneExpired(now)
		l.lastCleanup = now
	}

	identifierKey := limiterKey(l.key, path+"\x00identifier\x00"+canonicalIdentifier)
	globalKey := limiterKey(l.key, path+"\x00global")
	identifierBucket, hasIdentifier := l.buckets[identifierKey]
	globalBucket, hasGlobal := l.buckets[globalKey]
	missingBuckets := 0
	if !hasIdentifier {
		missingBuckets++
	}
	if !hasGlobal {
		missingBuckets++
	}
	if len(l.buckets)+missingBuckets > maxPublicPrivacyRateLimitKeys {
		return false, time.Minute - now.Sub(l.lastCleanup)
	}

	if hasIdentifier {
		var retryAfter time.Duration
		identifierBucket, retryAfter = pruneRateLimitBucket(identifierBucket, now)
		l.buckets[identifierKey] = identifierBucket
		if retryAfter > 0 {
			return false, retryAfter
		}
	}
	if hasGlobal {
		var retryAfter time.Duration
		globalBucket, retryAfter = pruneRateLimitBucket(globalBucket, now)
		l.buckets[globalKey] = globalBucket
		if retryAfter > 0 {
			return false, retryAfter
		}
	}

	if !hasIdentifier {
		identifierBucket = publicPrivacyRateLimitBucket{window: identifierWindow, limit: identifierLimit}
	}
	if !hasGlobal {
		globalBucket = publicPrivacyRateLimitBucket{window: globalWindow, limit: globalLimit}
	}
	identifierBucket.requests = append(identifierBucket.requests, now)
	globalBucket.requests = append(globalBucket.requests, now)
	l.buckets[identifierKey] = identifierBucket
	l.buckets[globalKey] = globalBucket
	return true, 0
}

func (l *publicPrivacyRateLimiter) pruneExpired(now time.Time) {
	for key, bucket := range l.buckets {
		if len(bucket.requests) == 0 || !bucket.requests[len(bucket.requests)-1].Add(bucket.window).After(now) {
			delete(l.buckets, key)
		}
	}
}

func pruneRateLimitBucket(bucket publicPrivacyRateLimitBucket, now time.Time) (publicPrivacyRateLimitBucket, time.Duration) {
	// Request times are ordered by allow's clock clamp. Drop only the expired prefix.
	cutoff := now.Add(-bucket.window)
	for len(bucket.requests) > 0 && !bucket.requests[0].After(cutoff) {
		bucket.requests = bucket.requests[1:]
	}
	if len(bucket.requests) < bucket.limit {
		return bucket, 0
	}
	return bucket, bucket.requests[0].Add(bucket.window).Sub(now)
}

func limiterKey(key [32]byte, value string) [32]byte {
	mac := hmac.New(sha256.New, key[:])
	_, _ = mac.Write([]byte(value))
	var digest [32]byte
	copy(digest[:], mac.Sum(nil))
	return digest
}
