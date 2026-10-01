package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// wsTicketTTL is how long a ticket can be redeemed; the client opens the socket right
// after requesting it.
const wsTicketTTL = 30 * time.Second

// wsTickets holds one-time tickets for opening the CRM WebSocket, so browsers don't have
// to put the API key in the socket URL (where it lands in proxy and access logs).
type wsTickets struct {
	mu sync.Mutex
	m  map[string]wsTicket
}

type wsTicket struct {
	username string
	expires  time.Time
}

// IssueWsTicket returns a single-use ticket for username, valid for wsTicketTTL.
func (c *Core) IssueWsTicket(username string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate ticket: %w", err)
	}
	ticket := hex.EncodeToString(b)
	now := time.Now()

	c.wsTickets.mu.Lock()
	defer c.wsTickets.mu.Unlock()
	for t, v := range c.wsTickets.m {
		if now.After(v.expires) {
			delete(c.wsTickets.m, t)
		}
	}
	c.wsTickets.m[ticket] = wsTicket{username: username, expires: now.Add(wsTicketTTL)}
	return ticket, nil
}

// RedeemWsTicket consumes a ticket and returns its username.
func (c *Core) RedeemWsTicket(ticket string) (string, error) {
	c.wsTickets.mu.Lock()
	t, ok := c.wsTickets.m[ticket]
	delete(c.wsTickets.m, ticket)
	c.wsTickets.mu.Unlock()

	if !ok || time.Now().After(t.expires) {
		return "", fmt.Errorf("invalid or expired ticket")
	}
	return t.username, nil
}
