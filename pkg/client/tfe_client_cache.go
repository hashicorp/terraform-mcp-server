// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"container/list"
	"crypto/sha256"
	"sync"
	"time"

	"github.com/hashicorp/go-tfe"
	log "github.com/sirupsen/logrus"
)

const (
	// statelessClientCacheSize caps how many clients we keep, one per token.
	statelessClientCacheSize = 1000
	// statelessClientIdleTTL drops clients nobody has used in a while.
	statelessClientIdleTTL = 10 * time.Minute
)

// statelessTfeClients caches clients for requests without a session ID.
// Building a client pings HCPT, so without this every stateless request paid
// for an extra round trip before the tool even ran.
var statelessTfeClients = newTfeClientCache(statelessClientCacheSize, statelessClientIdleTTL)

// statelessTfeClient returns a cached client for this address, TLS setting and
// token, or builds one and caches it.
func statelessTfeClient(terraformAddress string, terraformSkipTLSVerify bool, terraformToken string, logger *log.Logger) (*tfe.Client, error) {
	if terraformToken == "" {
		// Nothing to cache, newTfeClient returns the "no token" error
		return newTfeClient(terraformAddress, terraformSkipTLSVerify, terraformToken, logger)
	}
	key := tfeClientCacheKey(terraformAddress, terraformSkipTLSVerify, terraformToken)
	if client, ok := statelessTfeClients.get(key); ok {
		return client, nil
	}
	client, err := newTfeClient(terraformAddress, terraformSkipTLSVerify, terraformToken, logger)
	if err != nil {
		return nil, err
	}
	statelessTfeClients.add(key, client)
	logger.Debug("Created and cached a stateless TFE client")
	return client, nil
}

// tfeClientCacheKey hashes everything the client is built from, so a client
// only ever goes to requests with the exact same token, address and TLS setting.
func tfeClientCacheKey(terraformAddress string, terraformSkipTLSVerify bool, terraformToken string) [32]byte {
	skip := byte(0)
	if terraformSkipTLSVerify {
		skip = 1
	}
	b := make([]byte, 0, len(terraformAddress)+len(terraformToken)+3)
	b = append(b, terraformAddress...)
	b = append(b, 0, skip, 0)
	b = append(b, terraformToken...)
	return sha256.Sum256(b)
}

// tfeClientCache is an LRU that also drops idle clients. It's capped so lots
// of different tokens can't keep growing memory.
type tfeClientCache struct {
	mu      sync.Mutex
	maxSize int
	idleTTL time.Duration
	now     func() time.Time
	order   *list.List // the front is the most recently used
	entries map[[32]byte]*list.Element
}

type tfeClientCacheEntry struct {
	key      [32]byte
	client   *tfe.Client
	lastUsed time.Time
}

func newTfeClientCache(maxSize int, idleTTL time.Duration) *tfeClientCache {
	return &tfeClientCache{
		maxSize: maxSize,
		idleTTL: idleTTL,
		now:     time.Now,
		order:   list.New(),
		entries: map[[32]byte]*list.Element{},
	}
}

func (c *tfeClientCache) get(key [32]byte) (*tfe.Client, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	entry := elem.Value.(*tfeClientCacheEntry)
	if c.now().Sub(entry.lastUsed) > c.idleTTL {
		c.remove(elem)
		return nil, false
	}
	entry.lastUsed = c.now()
	c.order.MoveToFront(elem)
	return entry.client, true
}

func (c *tfeClientCache) add(key [32]byte, client *tfe.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[key]; ok {
		entry := elem.Value.(*tfeClientCacheEntry)
		entry.client = client
		entry.lastUsed = c.now()
		c.order.MoveToFront(elem)
		return
	}
	c.entries[key] = c.order.PushFront(&tfeClientCacheEntry{key: key, client: client, lastUsed: c.now()})

	// we drop anything that's gone idle, then the least recently used if we're
	// still over the limit
	for back := c.order.Back(); back != nil; back = c.order.Back() {
		entry := back.Value.(*tfeClientCacheEntry)
		if c.order.Len() <= c.maxSize && c.now().Sub(entry.lastUsed) <= c.idleTTL {
			break
		}
		c.remove(back)
	}
}

func (c *tfeClientCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

func (c *tfeClientCache) remove(elem *list.Element) {
	delete(c.entries, elem.Value.(*tfeClientCacheEntry).key)
	c.order.Remove(elem)
}
