package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// Subscription is a browser push subscription (push_subscriptions row).
type Subscription struct {
	ID       string
	UserID   string
	Endpoint string
	P256dh   string
	Auth     string
}

// Store gives the sender access to the subscriptions (implemented by the app
// on top of PocketBase).
type Store interface {
	// Targets returns the subscriptions of the users whose preferences allow c.
	Targets(userIDs []string, c Category) ([]Subscription, error)
	// Delivered records a successful delivery.
	Delivered(subID string)
	// Failed records a failure; gone = the push service says the
	// subscription no longer exists (404 / 410) → delete it.
	Failed(subID string, gone bool)
}

// Keys is the VAPID key pair (base64 URL, as produced by GenerateKeys).
type Keys struct {
	Public  string
	Private string
	// Subject is the contact of the application server (e-mail or https URL).
	Subject string
}

// GenerateKeys returns a new VAPID key pair.
func GenerateKeys() (Keys, error) {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return Keys{}, err
	}
	return Keys{Public: pub, Private: priv}, nil
}

// Pusher delivers one encrypted message (webpush by default, a fake in tests).
type Pusher interface {
	Push(ctx context.Context, sub Subscription, payload []byte, opts PushOptions) (status int, err error)
}

// PushOptions are the per-message delivery options.
type PushOptions struct {
	TTL     time.Duration
	Urgency string
	Topic   string
}

// WebPusher sends through the real Web Push protocol (RFC 8291 + VAPID).
type WebPusher struct {
	Keys   Keys
	Client *http.Client
}

// Push encrypts and posts the payload to the subscription endpoint.
func (w WebPusher) Push(ctx context.Context, sub Subscription, payload []byte, opts PushOptions) (int, error) {
	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	ttl := int(opts.TTL / time.Second)
	if ttl <= 0 {
		ttl = 3600
	}
	res, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		HTTPClient:      client,
		Subscriber:      strings.TrimPrefix(w.Keys.Subject, "mailto:"),
		VAPIDPublicKey:  w.Keys.Public,
		VAPIDPrivateKey: w.Keys.Private,
		TTL:             ttl,
		Urgency:         webpush.Urgency(opts.Urgency),
		Topic:           opts.Topic,
	})
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4<<10))
	return res.StatusCode, nil
}

// Sender queues messages and delivers them with a few workers, so that
// requests never wait for a push service.
type Sender struct {
	store  Store
	pusher Pusher
	log    *slog.Logger
	now    func() time.Time

	queue   chan job
	pending sync.WaitGroup
	workers sync.WaitGroup
	once    sync.Once
	mu      sync.RWMutex
	closed  bool
	ctx     context.Context
	cancel  context.CancelFunc
}

type job struct {
	users []string
	msg   Message
}

// Options configure a Sender.
type Options struct {
	Workers   int // default 4
	QueueSize int // default 256
	Logger    *slog.Logger
	Now       func() time.Time
}

// NewSender starts the worker pool.
func NewSender(store Store, pusher Pusher, o Options) *Sender {
	if o.Workers <= 0 {
		o.Workers = 4
	}
	if o.QueueSize <= 0 {
		o.QueueSize = 256
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Sender{store: store, pusher: pusher, log: o.Logger, now: o.Now, queue: make(chan job, o.QueueSize), ctx: ctx, cancel: cancel}
	for i := 0; i < o.Workers; i++ {
		s.workers.Add(1)
		go s.work()
	}
	return s
}

// ErrQueueFull is returned when the queue is saturated (message dropped).
var ErrQueueFull = errors.New("notify: file d'attente pleine")

// Send queues msg for users (never blocks). Empty user lists are ignored.
func (s *Sender) Send(users []string, msg Message) error {
	if s == nil || len(users) == 0 {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil
	}
	s.pending.Add(1)
	select {
	case s.queue <- job{users: append([]string(nil), users...), msg: msg}:
		return nil
	default:
		s.pending.Done()
		s.log.Warn("push: file d'attente pleine, notification ignorée", "kind", msg.Kind)
		return ErrQueueFull
	}
}

// Wait blocks until every queued message has been processed (tests).
func (s *Sender) Wait() {
	if s != nil {
		s.pending.Wait()
	}
}

// Close stops the workers after the queue is drained (or 5 s).
func (s *Sender) Close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		close(s.queue)
		s.mu.Unlock()
		done := make(chan struct{})
		go func() { s.workers.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			s.cancel()
			<-done
		}
		s.cancel()
	})
}

func (s *Sender) work() {
	defer s.workers.Done()
	for j := range s.queue {
		s.deliver(j)
		s.pending.Done()
	}
}

func (s *Sender) deliver(j job) {
	subs, err := s.store.Targets(j.users, j.msg.Category)
	if err != nil {
		s.log.Warn("push: lecture des abonnements", "error", err)
		return
	}
	if len(subs) == 0 {
		return
	}
	payload := j.msg.JSON(s.now())
	opts := PushOptions{TTL: j.msg.TTL, Urgency: j.msg.Urgency, Topic: j.msg.Topic}
	for _, sub := range subs {
		ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
		status, err := s.pusher.Push(ctx, sub, payload, opts)
		cancel()
		switch {
		case err != nil:
			s.log.Debug("push: envoi en échec", "error", err)
			s.store.Failed(sub.ID, false)
		case status == http.StatusNotFound || status == http.StatusGone:
			s.store.Failed(sub.ID, true)
		case status >= 200 && status < 300:
			s.store.Delivered(sub.ID)
		default:
			s.log.Debug("push: refus du service", "status", status)
			s.store.Failed(sub.ID, false)
		}
	}
}
