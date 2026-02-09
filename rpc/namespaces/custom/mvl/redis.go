package mvl

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"sort"
)

type redisPubSub struct {
	client *redis.Client
}

var (
	redisOnce sync.Once
	redisPS   *redisPubSub
	redisErr  error

	memOnce sync.Once
	mem     *memStore
)

type memStore struct {
	mu      sync.RWMutex
	kv      map[string][]byte
	sets    map[string]map[string]struct{}
	zsets   map[string]map[string]struct{}
	subs    map[string]map[int]func(string)
	nextSub int
}

func getMemStore() *memStore {
	memOnce.Do(func() {
		mem = &memStore{
			kv:    make(map[string][]byte),
			sets:  make(map[string]map[string]struct{}),
			zsets: make(map[string]map[string]struct{}),
			subs:  make(map[string]map[int]func(string)),
		}
	})
	return mem
}

func useInMemory() bool {
	if v := strings.ToLower(os.Getenv("MVL_STORAGE")); v == "redis" {
		return false
	}
	if v := strings.ToLower(os.Getenv("MVL_INMEMORY")); v == "0" || v == "false" || v == "no" {
		return false
	}
	// Default to in-memory unless explicitly forced to Redis.
	return true
}

func getRedisPubSub() (*redisPubSub, error) {
	redisOnce.Do(func() {
		client, err := newRedisClientFromEnv()
		if err != nil {
			redisErr = err
			return
		}
		redisPS = &redisPubSub{client: client}
	})
	if redisErr != nil {
		return nil, redisErr
	}
	return redisPS, nil
}

func newRedisClientFromEnv() (*redis.Client, error) {
	if url := os.Getenv("MVL_REDIS_URL"); url != "" {
		opts, err := redis.ParseURL(url)
		if err != nil {
			return nil, err
		}
		return redis.NewClient(opts), nil
	}

	addr := os.Getenv("MVL_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}

	db := 0
	if raw := os.Getenv("MVL_REDIS_DB"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid MVL_REDIS_DB: %w", err)
		}
		db = parsed
	}

	return redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("MVL_REDIS_PASSWORD"),
		DB:       db,
	}), nil
}

func PublishRedis(ctx context.Context, topic string, payload any) error {
	if useInMemory() {
		var data string
		switch v := payload.(type) {
		case string:
			data = v
		case []byte:
			data = string(v)
		default:
			bz, err := json.Marshal(v)
			if err != nil {
				return err
			}
			data = string(bz)
		}
		ms := getMemStore()
		ms.mu.RLock()
		defer ms.mu.RUnlock()
		for _, h := range ms.subs[topic] {
			h(data)
		}
		return nil
	}

	ps, err := getRedisPubSub()
	if err != nil {
		return err
	}

	var data []byte
	switch v := payload.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		data, err = json.Marshal(v)
		if err != nil {
			return err
		}
	}

	return ps.client.Publish(ctx, topic, data).Err()
}

func SubscribeRedis(ctx context.Context, topic string, handler func(payload string)) (func(), error) {
	if useInMemory() {
		ms := getMemStore()
		ms.mu.Lock()
		id := ms.nextSub
		ms.nextSub++
		if ms.subs[topic] == nil {
			ms.subs[topic] = make(map[int]func(string))
		}
		ms.subs[topic][id] = handler
		ms.mu.Unlock()
		return func() {
			ms.mu.Lock()
			if ms.subs[topic] != nil {
				delete(ms.subs[topic], id)
			}
			ms.mu.Unlock()
		}, nil
	}

	ps, err := getRedisPubSub()
	if err != nil {
		return nil, err
	}

	pubsub := ps.client.Subscribe(ctx, topic)
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return nil, err
	}

	ch := pubsub.Channel()
	go func() {
		for msg := range ch {
			handler(msg.Payload)
		}
	}()

	return func() {
		_ = pubsub.Close()
	}, nil
}

func SetRedis(ctx context.Context, key string, payload any) error {
	if useInMemory() {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		ms := getMemStore()
		ms.mu.Lock()
		ms.kv[key] = data
		ms.mu.Unlock()
		return nil
	}

	ps, err := getRedisPubSub()
	if err != nil {
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return ps.client.Set(ctx, key, data, 0).Err()
}

func GetRedis(ctx context.Context, key string) ([]byte, error) {
	if useInMemory() {
		ms := getMemStore()
		ms.mu.RLock()
		defer ms.mu.RUnlock()
		val, ok := ms.kv[key]
		if !ok {
			return nil, redis.Nil
		}
		cp := make([]byte, len(val))
		copy(cp, val)
		return cp, nil
	}

	ps, err := getRedisPubSub()
	if err != nil {
		return nil, err
	}
	return ps.client.Get(ctx, key).Bytes()
}

func SAddRedis(ctx context.Context, key string, members ...string) error {
	if useInMemory() {
		ms := getMemStore()
		ms.mu.Lock()
		set := ms.sets[key]
		if set == nil {
			set = make(map[string]struct{})
			ms.sets[key] = set
		}
		for _, m := range members {
			set[m] = struct{}{}
		}
		ms.mu.Unlock()
		return nil
	}

	ps, err := getRedisPubSub()
	if err != nil {
		return err
	}
	return ps.client.SAdd(ctx, key, members).Err()
}

func SRemRedis(ctx context.Context, key string, members ...string) error {
	if useInMemory() {
		ms := getMemStore()
		ms.mu.Lock()
		set := ms.sets[key]
		if set != nil {
			for _, m := range members {
				delete(set, m)
			}
		}
		ms.mu.Unlock()
		return nil
	}

	ps, err := getRedisPubSub()
	if err != nil {
		return err
	}
	return ps.client.SRem(ctx, key, members).Err()
}

func SScanRedis(ctx context.Context, key, cursor string, count int64) ([]string, string, error) {
	if useInMemory() {
		ms := getMemStore()
		ms.mu.RLock()
		set := ms.sets[key]
		items := make([]string, 0, len(set))
		for k := range set {
			items = append(items, k)
		}
		ms.mu.RUnlock()
		sort.Strings(items)
		start, err := strconv.Atoi(cursor)
		if err != nil || start < 0 {
			start = 0
		}
		if count <= 0 {
			count = 20
		}
		end := start + int(count)
		if start >= len(items) {
			return []string{}, "0", nil
		}
		if end > len(items) {
			end = len(items)
		}
		next := "0"
		if end < len(items) {
			next = strconv.Itoa(end)
		}
		return items[start:end], next, nil
	}

	ps, err := getRedisPubSub()
	if err != nil {
		return nil, "0", err
	}
	cur, err := strconv.ParseUint(cursor, 10, 64)
	if err != nil {
		cur = 0
	}
	keys, next, err := ps.client.SScan(ctx, key, cur, "", count).Result()
	return keys, strconv.FormatUint(next, 10), err
}

func ZAddRedis(ctx context.Context, key string, score float64, member string) error {
	if useInMemory() {
		ms := getMemStore()
		ms.mu.Lock()
		set := ms.zsets[key]
		if set == nil {
			set = make(map[string]struct{})
			ms.zsets[key] = set
		}
		set[member] = struct{}{}
		ms.mu.Unlock()
		return nil
	}

	ps, err := getRedisPubSub()
	if err != nil {
		return err
	}
	return ps.client.ZAdd(ctx, key, redis.Z{Score: score, Member: member}).Err()
}

func ZRemRedis(ctx context.Context, key string, members ...string) error {
	if useInMemory() {
		ms := getMemStore()
		ms.mu.Lock()
		set := ms.zsets[key]
		if set != nil {
			for _, m := range members {
				delete(set, m)
			}
		}
		ms.mu.Unlock()
		return nil
	}

	ps, err := getRedisPubSub()
	if err != nil {
		return err
	}
	return ps.client.ZRem(ctx, key, members).Err()
}

func ZRevRangeRedis(ctx context.Context, key string, start, stop int64) ([]string, error) {
	if useInMemory() {
		ms := getMemStore()
		ms.mu.RLock()
		set := ms.zsets[key]
		items := make([]string, 0, len(set))
		for m := range set {
			items = append(items, m)
		}
		ms.mu.RUnlock()
		sort.Sort(sort.Reverse(sort.StringSlice(items)))
		if start < 0 {
			start = 0
		}
		if stop < 0 {
			return []string{}, nil
		}
		if int(start) >= len(items) {
			return []string{}, nil
		}
		if int(stop) >= len(items) {
			stop = int64(len(items) - 1)
		}
		return items[start : stop+1], nil
	}

	ps, err := getRedisPubSub()
	if err != nil {
		return nil, err
	}
	return ps.client.ZRevRange(ctx, key, start, stop).Result()
}

func NowUnixMilli() int64 {
	return time.Now().UnixMilli()
}
