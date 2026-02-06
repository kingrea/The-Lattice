package eventbridge

import (
	"testing"
)

func TestRouterBuffersAndFlushes(t *testing.T) {
	router := NewRouter(RouterWithSubscriberCapacity(4))
	first := Event{EventID: "evt-1", ModuleID: "alpha", Type: "model_response"}
	second := Event{EventID: "evt-2", ModuleID: "alpha", Type: "tool.result"}
	router.Route(first)
	router.Route(second)
	sub := router.Subscribe("alpha")
	defer sub.Close()
	got1 := <-sub.Events
	if got1.EventID != first.EventID {
		t.Fatalf("expected first buffered event, got %s", got1.EventID)
	}
	got2 := <-sub.Events
	if got2.EventID != second.EventID {
		t.Fatalf("expected second buffered event, got %s", got2.EventID)
	}
}

func TestRouterDedupeByEventID(t *testing.T) {
	router := NewRouter()
	sub := router.Subscribe("alpha")
	defer sub.Close()
	event := Event{EventID: "evt-1", ModuleID: "alpha", Type: "model_response"}
	router.Route(event)
	router.Route(event)
	select {
	case got := <-sub.Events:
		if got.EventID != event.EventID {
			t.Fatalf("unexpected event: %s", got.EventID)
		}
	default:
		t.Fatalf("expected first delivery")
	}
	select {
	case <-sub.Events:
		t.Fatalf("duplicate event delivered")
	default:
	}
}

func TestRouterDropsOldestPreferredEventOnOverflow(t *testing.T) {
	router := NewRouter(RouterWithSubscriberCapacity(1))
	sub := router.Subscribe("alpha")
	defer sub.Close()
	oldest := Event{EventID: "evt-1", ModuleID: "alpha", Type: "model_response"}
	critical := Event{EventID: "evt-2", ModuleID: "alpha", Type: "session_end"}
	router.Route(oldest)
	router.Route(critical)
	if got := <-sub.Events; got.EventID != critical.EventID {
		t.Fatalf("expected critical event to replace oldest, got %s", got.EventID)
	}
}

func TestRouterDropsIncomingWhenOldestCritical(t *testing.T) {
	router := NewRouter(RouterWithSubscriberCapacity(1))
	sub := router.Subscribe("alpha")
	defer sub.Close()
	oldest := Event{EventID: "evt-1", ModuleID: "alpha", Type: "session_end"}
	droppable := Event{EventID: "evt-2", ModuleID: "alpha", Type: "model_response"}
	router.Route(oldest)
	router.Route(droppable)
	if got := <-sub.Events; got.EventID != oldest.EventID {
		t.Fatalf("expected oldest critical event to remain, got %s", got.EventID)
	}
	select {
	case <-sub.Events:
		t.Fatalf("unexpected extra event")
	default:
	}
}
