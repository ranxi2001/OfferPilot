package session

import (
	"fmt"
	"sync"
	"testing"
)

func TestMemoryIsolationAndDefensiveCopies(t *testing.T) {
	t.Parallel()

	memory := NewMemory(0)
	metadata := map[string]string{"source": "resume-a"}
	if err := memory.Append("candidate-a", Message{Role: "user", Content: "project A", Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	metadata["source"] = "mutated"
	if err := memory.Add("candidate-b", "user", "project B"); err != nil {
		t.Fatal(err)
	}

	a := memory.Messages("candidate-a")
	b := memory.Messages("candidate-b")
	if len(a) != 1 || a[0].Content != "project A" || a[0].Metadata["source"] != "resume-a" {
		t.Fatalf("candidate A memory was not isolated: %#v", a)
	}
	if len(b) != 1 || b[0].Content != "project B" {
		t.Fatalf("candidate B memory was not isolated: %#v", b)
	}
	a[0].Content = "changed outside"
	a[0].Metadata["source"] = "changed outside"
	stored := memory.Messages("candidate-a")
	if stored[0].Content != "project A" || stored[0].Metadata["source"] != "resume-a" {
		t.Fatalf("returned state aliases stored state: %#v", stored)
	}
}

func TestMemoryConcurrentSessions(t *testing.T) {
	t.Parallel()

	memory := NewMemory(200)
	var wait sync.WaitGroup
	for candidate := 0; candidate < 8; candidate++ {
		candidate := candidate
		wait.Add(1)
		go func() {
			defer wait.Done()
			id := fmt.Sprintf("candidate-%d", candidate)
			for message := 0; message < 100; message++ {
				if err := memory.Add(id, "user", fmt.Sprintf("%d", message)); err != nil {
					t.Errorf("Append: %v", err)
					return
				}
			}
		}()
	}
	wait.Wait()
	for candidate := 0; candidate < 8; candidate++ {
		id := fmt.Sprintf("candidate-%d", candidate)
		if got := len(memory.Messages(id)); got != 100 {
			t.Fatalf("%s has %d messages, want 100", id, got)
		}
	}
}
