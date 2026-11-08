package main

import (
	"fmt"
	"testing"
)

func TestOpenLinksBoundedValidatedAndConsumed(t *testing.T) {
	openLinks.Lock()
	openLinks.queue = nil
	openLinks.Unlock()
	t.Cleanup(func() { openLinks.Lock(); openLinks.queue = nil; openLinks.Unlock() })
	service := &DesktopService{}
	queueOpenLink("skillhub://open?hub=https://example.com&skill=team/demo&version=latest")
	if service.NextOpenRequest() != nil {
		t.Fatal("invalid link entered queue")
	}
	link := "skillhub://open?hub=https://example.com&skill=team/demo&version=1.0.0"
	queueOpenLink(link)
	queueOpenLink(link)
	if service.NextOpenRequest() == nil || service.NextOpenRequest() != nil {
		t.Fatal("queue failed to deduplicate or consume")
	}
	for i := 0; i < 20; i++ {
		queueOpenLink(fmt.Sprintf("skillhub://open?hub=https://example.com&skill=team/demo&version=1.0.%d", i))
	}
	for i := 0; i < 8; i++ {
		if r := service.NextOpenRequest(); r == nil || r.Version != fmt.Sprintf("1.0.%d", i) {
			t.Fatal("bounded queue lost order")
		}
	}
	if service.NextOpenRequest() != nil {
		t.Fatal("queue exceeded limit")
	}
}
