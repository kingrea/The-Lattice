package eventbridge

import (
	"strings"
	"sync"
)

const (
	defaultSubscriberCapacity = 100
	defaultBacklogLimit       = 50
	defaultDedupeWindow       = 1024
)

// RouterOption customizes Router construction.
type RouterOption func(*Router)

// Router delivers bridge events to module-specific subscribers with buffering,
// deduplication, and bounded channel semantics.
type Router struct {
	mu             sync.RWMutex
	subscribers    map[string]map[*subscriber]struct{}
	backlog        map[string][]Event
	sessionModules map[string]string
	recentIDs      map[string]struct{}
	recentOrder    []string
	channelSize    int
	backlogLimit   int
	dedupeWindow   int
	logger         Logger
}

// Subscription represents an active module subscription.
type Subscription struct {
	Events <-chan Event
	cancel func()
}

// Close terminates the subscription.
func (s Subscription) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

// NewRouter constructs a router with sane defaults.
func NewRouter(opts ...RouterOption) *Router {
	r := &Router{
		subscribers:    map[string]map[*subscriber]struct{}{},
		backlog:        map[string][]Event{},
		sessionModules: map[string]string{},
		recentIDs:      map[string]struct{}{},
		recentOrder:    make([]string, 0, defaultDedupeWindow),
		channelSize:    defaultSubscriberCapacity,
		backlogLimit:   defaultBacklogLimit,
		dedupeWindow:   defaultDedupeWindow,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	return r
}

// WithLogger injects a logger for drop/diagnostic messages.
func RouterWithLogger(logger Logger) RouterOption {
	return func(r *Router) {
		r.logger = logger
	}
}

// WithSubscriberCapacity overrides the buffered channel size per subscriber.
func RouterWithSubscriberCapacity(cap int) RouterOption {
	return func(r *Router) {
		if cap > 0 {
			r.channelSize = cap
		}
	}
}

// WithBacklogLimit overrides the backlog size for pre-subscription buffering.
func RouterWithBacklogLimit(limit int) RouterOption {
	return func(r *Router) {
		if limit > 0 {
			r.backlogLimit = limit
		}
	}
}

// WithDedupeWindow controls how many recent event IDs are retained.
func RouterWithDedupeWindow(size int) RouterOption {
	return func(r *Router) {
		if size > 0 {
			r.dedupeWindow = size
		}
	}
}

// Subscribe registers for events keyed by module ID.
func (r *Router) Subscribe(moduleID string) Subscription {
	module := normalizeModule(moduleID)
	sub := newSubscriber(r.channelSize, r.logger)
	var backlog []Event
	r.mu.Lock()
	if r.subscribers[module] == nil {
		r.subscribers[module] = map[*subscriber]struct{}{}
	}
	r.subscribers[module][sub] = struct{}{}
	if existing := r.backlog[module]; len(existing) > 0 {
		backlog = append(backlog, existing...)
		delete(r.backlog, module)
	}
	r.mu.Unlock()
	for _, event := range backlog {
		sub.deliver(event)
	}
	return Subscription{
		Events: sub.channel(),
		cancel: func() {
			r.removeSubscriber(module, sub)
		},
	}
}

// HandleEvent satisfies the EventProcessor interface.
func (r *Router) HandleEvent(event Event) error {
	r.Route(event)
	return nil
}

// Route delivers the event to subscribers or buffers it when no subscriber exists.
func (r *Router) Route(event Event) {
	if event.EventID != "" && r.isDuplicate(event.EventID) {
		return
	}
	module := normalizeModule(event.ModuleID)
	if module == "" {
		module = r.lookupModule(event.SessionID)
	}
	if module == "" {
		return
	}
	r.trackSession(event.SessionID, module)
	r.mu.RLock()
	subs := r.snapshotSubscribers(module)
	r.mu.RUnlock()
	if len(subs) == 0 {
		r.bufferEvent(module, event)
		return
	}
	for _, sub := range subs {
		sub.deliver(event)
	}
}

func (r *Router) snapshotSubscribers(module string) []*subscriber {
	live := r.subscribers[module]
	if len(live) == 0 {
		return nil
	}
	items := make([]*subscriber, 0, len(live))
	for sub := range live {
		items = append(items, sub)
	}
	return items
}

func (r *Router) removeSubscriber(module string, sub *subscriber) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if subs := r.subscribers[module]; subs != nil {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(r.subscribers, module)
		}
	}
	sub.close()
}

func (r *Router) bufferEvent(module string, event Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	queue := r.backlog[module]
	if len(queue) >= r.backlogLimit {
		queue = queue[1:]
		if r.logger != nil {
			r.logger.Printf("eventbridge: backlog drop for %s (limit %d)", module, r.backlogLimit)
		}
	}
	queue = append(queue, event)
	r.backlog[module] = queue
}

func (r *Router) trackSession(sessionID, moduleID string) {
	if sessionID == "" || moduleID == "" {
		return
	}
	r.mu.Lock()
	r.sessionModules[sessionID] = moduleID
	r.mu.Unlock()
}

func (r *Router) lookupModule(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sessionModules[sessionID]
}

func (r *Router) isDuplicate(eventID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.recentIDs[eventID]; ok {
		return true
	}
	r.recentIDs[eventID] = struct{}{}
	r.recentOrder = append(r.recentOrder, eventID)
	if len(r.recentOrder) > r.dedupeWindow {
		oldest := r.recentOrder[0]
		r.recentOrder = r.recentOrder[1:]
		delete(r.recentIDs, oldest)
	}
	return false
}

func normalizeModule(moduleID string) string {
	return strings.TrimSpace(strings.ToLower(moduleID))
}

type subscriber struct {
	ch      chan Event
	logger  Logger
	closed  bool
	closeMu sync.Mutex
}

func newSubscriber(capacity int, logger Logger) *subscriber {
	if capacity <= 0 {
		capacity = defaultSubscriberCapacity
	}
	return &subscriber{
		ch:     make(chan Event, capacity),
		logger: logger,
	}
}

func (s *subscriber) channel() <-chan Event {
	return s.ch
}

func (s *subscriber) deliver(event Event) {
	if s.isClosed() {
		return
	}
	select {
	case s.ch <- event:
		return
	default:
		oldest := <-s.ch
		if shouldDropOldest(oldest, event) {
			s.logDrop(oldest, "queue overflow")
			s.ch <- event
		} else {
			s.ch <- oldest
			s.logDrop(event, "queue overflow:incoming")
		}
	}
}

func (s *subscriber) logDrop(event Event, reason string) {
	if s.logger == nil {
		return
	}
	s.logger.Printf("eventbridge: dropped %s (%s)", event.Type, reason)
}

func (s *subscriber) close() {
	s.closeMu.Lock()
	if s.closed {
		s.closeMu.Unlock()
		return
	}
	s.closed = true
	close(s.ch)
	s.closeMu.Unlock()
}

func (s *subscriber) isClosed() bool {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	return s.closed
}

func shouldDropOldest(oldest, incoming Event) bool {
	oldestCritical := isCriticalEvent(oldest.Type)
	incomingCritical := isCriticalEvent(incoming.Type)
	switch {
	case oldestCritical && !incomingCritical:
		return false
	case !oldestCritical && incomingCritical:
		return true
	}
	oldestPreferred := isPreferredDrop(oldest.Type)
	incomingPreferred := isPreferredDrop(incoming.Type)
	if oldestPreferred && !incomingPreferred {
		return true
	}
	if !oldestPreferred && incomingPreferred {
		return false
	}
	return true
}

func isCriticalEvent(kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	return kind == "session_end" || kind == "error"
}

func isPreferredDrop(kind string) bool {
	return strings.ToLower(strings.TrimSpace(kind)) == "model_response"
}
