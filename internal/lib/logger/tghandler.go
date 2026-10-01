package logger

import (
	"DarkCS/bot"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// tgQueueSize bounds memory during log bursts; records beyond it are dropped and
	// counted rather than blocking the code that logged them.
	tgQueueSize = 500
	// tgSendInterval paces sends: Telegram rate-limits each chat to about 1 message/s.
	tgSendInterval = time.Second
	// tgMaxMessage stays under Telegram's 4096-character message limit.
	tgMaxMessage = 3900
)

// TelegramHandler is a slog.Handler that writes every record to the wrapped handler and
// forwards records at or above minLevel to the admin Telegram bot.
//
// Forwarding is asynchronous: Handle only enqueues, and a single sender goroutine
// delivers, so logging never blocks a request on a Telegram API call. Consecutive
// records of the same level are batched into one message; on overflow records are
// dropped and the count is reported in the next message. The bot itself filters per
// admin (/level), so records are sent with their level.
type TelegramHandler struct {
	handler  slog.Handler
	sink     *tgSink
	minLevel slog.Level
	attrs    []slog.Attr
	group    string
}

type tgRecord struct {
	level slog.Level
	text  string
}

// tgSink is shared by a handler and all its WithAttrs/WithGroup clones.
type tgSink struct {
	bot     *bot.TgBot
	queue   chan tgRecord
	dropped atomic.Int64
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
}

// NewTelegramHandler creates a TelegramHandler and starts its sender goroutine.
func NewTelegramHandler(handler slog.Handler, bot *bot.TgBot, minLevel slog.Level) *TelegramHandler {
	sink := &tgSink{
		bot:   bot,
		queue: make(chan tgRecord, tgQueueSize),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
	go sink.run()
	return &TelegramHandler{handler: handler, sink: sink, minLevel: minLevel}
}

// Enabled reports whether the wrapped handler wants the record; the Telegram threshold
// is applied separately in Handle so it never hides records from the log file.
func (h *TelegramHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle writes the record to the wrapped handler and enqueues it for Telegram.
func (h *TelegramHandler) Handle(ctx context.Context, record slog.Record) error {
	if err := h.handler.Handle(ctx, record); err != nil {
		return err
	}
	if record.Level < h.minLevel || h.sink.bot == nil {
		return nil
	}

	var b strings.Builder
	if h.group != "" {
		fmt.Fprintf(&b, "[%s] %s.%s", record.Level.String(), h.group, record.Message)
	} else {
		fmt.Fprintf(&b, "[%s] %s", record.Level.String(), record.Message)
	}
	for _, attr := range h.attrs {
		fmt.Fprintf(&b, "\n%s: %v", attr.Key, attr.Value)
	}
	record.Attrs(func(attr slog.Attr) bool {
		fmt.Fprintf(&b, "\n%s: %v", attr.Key, attr.Value)
		return true
	})

	select {
	case h.sink.queue <- tgRecord{level: record.Level, text: truncate(b.String(), tgMaxMessage)}:
	default:
		h.sink.dropped.Add(1)
	}
	return nil
}

// WithAttrs implements slog.Handler.WithAttrs
func (h *TelegramHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &TelegramHandler{
		handler:  h.handler.WithAttrs(attrs),
		sink:     h.sink,
		minLevel: h.minLevel,
		attrs:    newAttrs,
		group:    h.group,
	}
}

// WithGroup implements slog.Handler.WithGroup
func (h *TelegramHandler) WithGroup(name string) slog.Handler {
	group := name
	if h.group != "" {
		group = h.group + "." + name
	}

	return &TelegramHandler{
		handler:  h.handler.WithGroup(name),
		sink:     h.sink,
		minLevel: h.minLevel,
		attrs:    h.attrs,
		group:    group,
	}
}

// Flush delivers queued records and stops the sender, or gives up when ctx expires.
// Records logged after Flush are written to the file only.
func (h *TelegramHandler) Flush(ctx context.Context) {
	h.sink.once.Do(func() { close(h.sink.stop) })
	select {
	case <-h.sink.done:
	case <-ctx.Done():
	}
}

func (s *tgSink) run() {
	defer close(s.done)
	ticker := time.NewTicker(tgSendInterval)
	defer ticker.Stop()

	var pending *tgRecord // record read while batching that didn't fit the batch
	for {
		var first tgRecord
		if pending != nil {
			first, pending = *pending, nil
		} else {
			select {
			case first = <-s.queue:
			case <-s.stop:
				s.drain()
				return
			}
		}

		batch, next := s.batch(first)
		pending = next
		s.send(batch)

		select {
		case <-ticker.C:
		case <-s.stop:
			if pending != nil {
				s.send(*pending)
			}
			s.drain()
			return
		}
	}
}

// batch appends immediately available records of the same level to first, up to the
// message size limit. A record that doesn't fit is returned for the next round.
func (s *tgSink) batch(first tgRecord) (tgRecord, *tgRecord) {
	for {
		select {
		case r := <-s.queue:
			if r.level != first.level || len(first.text)+len(r.text)+2 > tgMaxMessage {
				return first, &r
			}
			first.text += "\n\n" + r.text
		default:
			return first, nil
		}
	}
}

// drain sends whatever is still queued at shutdown, unpaced.
func (s *tgSink) drain() {
	for {
		select {
		case r := <-s.queue:
			s.send(r)
		default:
			return
		}
	}
}

func (s *tgSink) send(r tgRecord) {
	// A panic here would kill the only sender and silently stop admin alerts.
	defer func() { _ = recover() }()
	if n := s.dropped.Swap(0); n > 0 {
		r.text = fmt.Sprintf("(%d log messages dropped: queue full)\n\n%s", n, r.text)
	}
	s.bot.SendMessageWithLevel(r.text, r.level)
}

// truncate cuts s to at most n bytes on a rune boundary.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n] + "…"
}
