package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient manages Redis connections and operations
type RedisClient struct {
	client *redis.Client
}

var redisInstance *RedisClient

// InitRedis initializes the Redis client
func InitRedis() error {
	redisURL := os.Getenv("REDIS_URL")

	var client *redis.Client
	if redisURL != "" {
		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			return fmt.Errorf("failed to parse REDIS_URL: %w", err)
		}
		client = redis.NewClient(opt)
	} else {
		// Fallback to REDIS_ADDR/REDIS_PASSWORD
		client = redis.NewClient(&redis.Options{
			Addr:     getRedisAddr(),
			Password: getRedisPassword(),
			DB:       0,
			PoolSize: 10,
		})
	}

	redisInstance = &RedisClient{
		client: client,
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("Failed to connect to Redis: %v", err)
		return err
	}

	log.Println("Redis client initialized successfully")
	return nil
}

func getRedisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "localhost:6379"
}

func getRedisPassword() string {
	if pwd := os.Getenv("REDIS_PASSWORD"); pwd != "" {
		return pwd
	}
	return ""
}

// ============ STREAK TRACKING ============

// GetStreak retrieves the current streak for a user
func GetStreak(userID string) (int, error) {
	ctx := context.Background()
	key := fmt.Sprintf("streak:user:%s", userID)

	val, err := redisInstance.client.Get(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get streak: %w", err)
	}

	if val == "" {
		return 0, nil
	}

	streak, err := jsonToInt(val)
	if err != nil {
		return 0, err
	}

	return streak, nil
}

// SetStreak sets the streak for a user
func SetStreak(userID string, streak int) error {
	ctx := context.Background()
	key := fmt.Sprintf("streak:user:%s", userID)

	err := redisInstance.client.Set(ctx, key, streak, 0).Err()
	if err != nil {
		return fmt.Errorf("failed to set streak: %w", err)
	}

	// Set expiration to 30 days
	err = redisInstance.client.Expire(ctx, key, 30*24*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to set streak expiration: %w", err)
	}

	return nil
}

// IncrementDaily increments the daily activity counter
func IncrementDaily(userID string) (int, error) {
	ctx := context.Background()
	today := time.Now().Format("2006-01-02")
	key := fmt.Sprintf("streak:daily:%s:%s", userID, today)

	val, err := redisInstance.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to increment daily: %w", err)
	}

	// Set expiration to 2 days
	redisInstance.client.Expire(ctx, key, 48*time.Hour)

	return int(val), nil
}

// CheckDailyActivity checks if user was active today
func CheckDailyActivity(userID string) (bool, error) {
	ctx := context.Background()
	today := time.Now().Format("2006-01-02")
	key := fmt.Sprintf("streak:daily:%s:%s", userID, today)

	val, err := redisInstance.client.Get(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check daily activity: %w", err)
	}

	return val != "", nil
}

// UpdateLastActiveDate updates the last active date for a user
func UpdateLastActiveDate(userID string) error {
	ctx := context.Background()
	key := fmt.Sprintf("streak:last_active:%s", userID)
	today := time.Now().Format("2006-01-02")

	err := redisInstance.client.Set(ctx, key, today, 0).Err()
	if err != nil {
		return fmt.Errorf("failed to update last active date: %w", err)
	}

	return nil
}

// ============ SESSION TRACKING ============

// SessionKey generates the key for a quiz session
func SessionKey(sessionID string) string {
	return fmt.Sprintf("session:active:%s", sessionID)
}

// GetActiveSession retrieves an active session for a user
func GetActiveSession(userID string) (map[string]interface{}, error) {
	ctx := context.Background()
	key := fmt.Sprintf("session:active:user:%s", userID)

	val, err := redisInstance.client.Get(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get active session: %w", err)
	}

	if val == "" {
		return nil, nil
	}

	var sessionData map[string]interface{}
	err = json.Unmarshal([]byte(val), &sessionData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	return sessionData, nil
}

// SetActiveSession sets the active session for a user
func SetActiveSession(userID string, sessionID string, sessionData map[string]interface{}) error {
	ctx := context.Background()
	key := fmt.Sprintf("session:active:user:%s", userID)

	data, err := json.Marshal(sessionData)
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %w", err)
	}

	// Store session with 1 hour expiration
	err = redisInstance.client.Set(ctx, key, data, 1*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to set active session: %w", err)
	}

	return nil
}

// ClearActiveSession removes the active session for a user
func ClearActiveSession(userID string) error {
	ctx := context.Background()
	key := fmt.Sprintf("session:active:user:%s", userID)

	err := redisInstance.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to clear active session: %w", err)
	}

	return nil
}

// ============ RATE LIMITING ============

// RateLimitKey generates the key for rate limiting
func RateLimitKey(userID string, endpoint string) string {
	return fmt.Sprintf("ratelimit:%s:%s", userID, endpoint)
}

// CheckRateLimit checks if a user is rate limited for an endpoint
func CheckRateLimit(userID string, endpoint string, maxRequests int, window time.Duration) (bool, error) {
	ctx := context.Background()
	key := RateLimitKey(userID, endpoint)

	current, err := redisInstance.client.Get(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to get rate limit: %w", err)
	}

	if current == "" {
		// First request in window
		pipe := redisInstance.client.Pipeline()
		pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, window)
		_, err = pipe.Exec(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to set rate limit: %w", err)
		}
		return false, nil
	}

	count, err := jsonToInt(current)
	if err != nil {
		return false, err
	}

	return count < maxRequests, nil
}

// IncrementRateLimit increments the rate limit counter
func IncrementRateLimit(userID string, endpoint string) error {
	ctx := context.Background()
	key := RateLimitKey(userID, endpoint)

	_, err := redisInstance.client.Incr(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to increment rate limit: %w", err)
	}

	return nil
}

// ============ DISTRIBUTED LOCKS ============

// AcquireLock acquires a distributed lock for a given resource
func AcquireLock(resource string, expiration time.Duration) (bool, error) {
	ctx := context.Background()
	key := fmt.Sprintf("lock:%s", resource)

	// Use Redis SETNX for simple distributed locking
	result, err := redisInstance.client.SetNX(ctx, key, "locked", expiration).Result()
	if err != nil {
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}

	// If result is false, lock was already held
	if !result {
		return false, nil
	}

	return true, nil
}

// ReleaseLock releases a distributed lock
func ReleaseLock(resource string) error {
	ctx := context.Background()
	key := fmt.Sprintf("lock:%s", resource)

	_, err := redisInstance.client.Del(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	return nil
}

// ============ HELPER FUNCTIONS ============

// jsonToInt converts a Redis value to int
func jsonToInt(val interface{}) (int, error) {
	str, ok := val.(string)
	if !ok {
		return 0, fmt.Errorf("value is not a string")
	}

	var result int
	err := json.Unmarshal([]byte(str), &result)
	if err != nil {
		return 0, err
	}

	return result, nil
}

// ============ CACHE MANAGEMENT ============

// InvalidateUserCache clears all cache entries for a user
func InvalidateUserCache(userID string) error {
	ctx := context.Background()
	pattern := fmt.Sprintf("*user:%s*", userID)

	// Use Scan to find all matching keys
	var keys []string
	iter := redisInstance.client.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("failed to scan cache: %w", err)
	}

	// Delete all matching keys
	if len(keys) > 0 {
		_, err := redisInstance.client.Del(ctx, keys...).Result()
		if err != nil {
			return fmt.Errorf("failed to delete cache keys: %w", err)
		}
	}
	return nil
}

// SetCache sets a value in cache with expiration
func SetCache(key string, value interface{}, expiration time.Duration) error {
	ctx := context.Background()

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal cache value: %w", err)
	}

	err = redisInstance.client.Set(ctx, key, data, expiration).Err()
	if err != nil {
		return fmt.Errorf("failed to set cache: %w", err)
	}

	return nil
}

// GetCache retrieves a value from cache
func GetCache(key string) (interface{}, error) {
	ctx := context.Background()

	val, err := redisInstance.client.Get(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get cache: %w", err)
	}

	if val == "" {
		return nil, nil
	}

	var result interface{}
	err = json.Unmarshal([]byte(val), &result)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal cache value: %w", err)
	}

	return result, nil
}
