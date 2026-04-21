package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
}

func New(addr string) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &Client{rdb: rdb}, nil
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.rdb.Get(ctx, key).Result()
}

func (c *Client) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.rdb.TTL(ctx, key).Result()
}

func (c *Client) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c *Client) Del(ctx context.Context, keys ...string) error {
	return c.rdb.Del(ctx, keys...).Err()
}

func (c *Client) Exists(ctx context.Context, keys ...string) (int64, error) {
	return c.rdb.Exists(ctx, keys...).Result()
}

func (c *Client) SetNX(ctx context.Context, key string, value any, ttl time.Duration) (bool, error) {
	result, err := c.rdb.SetArgs(ctx, key, value, redis.SetArgs{Mode: "NX", TTL: ttl}).Result()
	return result == "OK", err
}

func (c *Client) HSet(ctx context.Context, key string, values ...any) error {
	return c.rdb.HSet(ctx, key, values...).Err()
}

func (c *Client) HGet(ctx context.Context, key, field string) (string, error) {
	return c.rdb.HGet(ctx, key, field).Result()
}

func (c *Client) SAdd(ctx context.Context, key string, members ...any) error {
	return c.rdb.SAdd(ctx, key, members...).Err()
}

func (c *Client) SRem(ctx context.Context, key string, members ...any) error {
	return c.rdb.SRem(ctx, key, members...).Err()
}

func (c *Client) SMembers(ctx context.Context, key string) ([]string, error) {
	return c.rdb.SMembers(ctx, key).Result()
}

func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	return c.rdb.Incr(ctx, key).Result()
}

func (c *Client) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.rdb.Expire(ctx, key, ttl).Err()
}

func (c *Client) RDB() *redis.Client {
	return c.rdb
}
