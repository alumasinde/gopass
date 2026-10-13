package ratelimit

import (
	"sync"
	"time"
)

const (
	MaxLoginAttempts    = 5
	LoginWindowDuration = 15 * time.Minute
	MaxPerIPAttempts    = 20
	MaxPerEmailAttempts = 10
)

type Attempt struct {
	Count     int
	FirstTime time.Time
}

type RateLimiter struct {
	mu            sync.RWMutex
	ipAttempts    map[string]*Attempt
	emailAttempts map[string]*Attempt
}

func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		ipAttempts:    make(map[string]*Attempt),
		emailAttempts: make(map[string]*Attempt),
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) CheckLogin(ip, email string) bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	now := time.Now()

	// Check IP rate limit
	if ipAttempt, exists := rl.ipAttempts[ip]; exists {
		if now.Sub(ipAttempt.FirstTime) < LoginWindowDuration {
			if ipAttempt.Count >= MaxPerIPAttempts {
				return false
			}
		}
	}

	// Check email rate limit
	if emailAttempt, exists := rl.emailAttempts[email]; exists {
		if now.Sub(emailAttempt.FirstTime) < LoginWindowDuration {
			if emailAttempt.Count >= MaxPerEmailAttempts {
				return false
			}
		}
	}

	return true
}

func (rl *RateLimiter) RecordLoginAttempt(ip, email string, success bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Record IP attempt
	if attempt, exists := rl.ipAttempts[ip]; exists {
		if now.Sub(attempt.FirstTime) >= LoginWindowDuration {
			// Reset if window expired
			attempt.Count = 1
			attempt.FirstTime = now
		} else {
			attempt.Count++
		}
	} else {
		rl.ipAttempts[ip] = &Attempt{Count: 1, FirstTime: now}
	}

	// Record email attempt
	if attempt, exists := rl.emailAttempts[email]; exists {
		if now.Sub(attempt.FirstTime) >= LoginWindowDuration {
			// Reset if window expired
			attempt.Count = 1
			attempt.FirstTime = now
		} else {
			attempt.Count++
		}
	} else {
		rl.emailAttempts[email] = &Attempt{Count: 1, FirstTime: now}
	}

	// Clear on successful login (optional - can keep for audit)
	if success {
		delete(rl.ipAttempts, ip)
		delete(rl.emailAttempts, email)
	}
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()

		// Clean up expired IP attempts
		for ip, attempt := range rl.ipAttempts {
			if now.Sub(attempt.FirstTime) >= LoginWindowDuration {
				delete(rl.ipAttempts, ip)
			}
		}

		// Clean up expired email attempts
		for email, attempt := range rl.emailAttempts {
			if now.Sub(attempt.FirstTime) >= LoginWindowDuration {
				delete(rl.emailAttempts, email)
			}
		}

		rl.mu.Unlock()
	}
}
