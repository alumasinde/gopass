package ratelimit

import (
	"fmt"
	"sync"
	"testing"
)

func TestEmailAttemptLimit(t *testing.T) {
	rl := &RateLimiter{ipAttempts: map[string]*Attempt{}, emailAttempts: map[string]*Attempt{}}
	for i := 0; i < MaxPerEmailAttempts; i++ {
		if !rl.CheckLogin("192.0.2.1", "person@example.test") {
			t.Fatalf("attempt %d unexpectedly blocked", i+1)
		}
		rl.RecordLoginAttempt("192.0.2.1", "person@example.test", false)
	}
	if rl.CheckLogin("192.0.2.1", "person@example.test") {
		t.Fatal("email should be blocked after configured attempt limit")
	}
}

func TestIPAttemptLimit(t *testing.T) {
	rl := &RateLimiter{ipAttempts: map[string]*Attempt{}, emailAttempts: map[string]*Attempt{}}
	for i := 0; i < MaxPerIPAttempts; i++ {
		email := fmt.Sprintf("person%d@example.test", i)
		if !rl.CheckLogin("192.0.2.2", email) {
			t.Fatalf("attempt %d unexpectedly blocked", i+1)
		}
		rl.RecordLoginAttempt("192.0.2.2", email, false)
	}
	if rl.CheckLogin("192.0.2.2", "new@example.test") {
		t.Fatal("IP should be blocked after configured attempt limit")
	}
}

func TestSuccessfulLoginClearsAttempts(t *testing.T) {
	rl := &RateLimiter{ipAttempts: map[string]*Attempt{}, emailAttempts: map[string]*Attempt{}}
	for i := 0; i < MaxPerEmailAttempts; i++ {
		rl.RecordLoginAttempt("192.0.2.3", "person@example.test", false)
	}
	if rl.CheckLogin("192.0.2.3", "person@example.test") {
		t.Fatal("expected limit before successful login reset")
	}
	rl.RecordLoginAttempt("192.0.2.3", "person@example.test", true)
	if !rl.CheckLogin("192.0.2.3", "person@example.test") {
		t.Fatal("successful login should clear in-memory attempts")
	}
}

func TestConcurrentRateLimitAccessIsRaceSafe(t *testing.T) {
	rl := &RateLimiter{ipAttempts: map[string]*Attempt{}, emailAttempts: map[string]*Attempt{}}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ip := fmt.Sprintf("192.0.2.%d", i+1)
			email := fmt.Sprintf("user%d@example.test", i)
			for j := 0; j < 10; j++ {
				_ = rl.CheckLogin(ip, email)
				rl.RecordLoginAttempt(ip, email, false)
			}
		}(i)
	}
	wg.Wait()
}
