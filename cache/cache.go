package cache

import (
	"time"

	"github.com/jellydator/ttlcache/v3"
)

type CacheService interface {
	Delete(key string)
	Get(key string) []byte
	Set(key string, data []byte, ttl time.Duration)
}

type Cache struct {
	cache *ttlcache.Cache[string, []byte]
}

func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		cache: ttlcache.New(ttlcache.WithTTL[string, []byte](ttl), ttlcache.WithDisableTouchOnHit[string, []byte]()),
	}
}

func (c *Cache) Delete(key string) {
	c.cache.Delete(key)
}

func (c *Cache) Get(key string) []byte {
	item := c.cache.Get(key)
	if item == nil || item.IsExpired() {
		return nil
	}
	return item.Value()
}

func (c *Cache) Set(key string, data []byte, ttl time.Duration) {
	c.cache.Set(key, data, ttl)
}
