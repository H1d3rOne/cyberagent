package builtin

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestTCPPortCheckOpen(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := TCPPortCheck(ctx, TCPPortCheckOptions{
		Host:    "127.0.0.1",
		Ports:   []int{port},
		Timeout: 2 * time.Second,
	}, "job_001", "step_port_001")

	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if !result.Results[0].Open {
		t.Errorf("expected port %d to be open", port)
	}
	if len(result.Evidence) == 0 {
		t.Error("expected evidence for open port")
	}
}

func TestTCPPortCheckClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := TCPPortCheck(ctx, TCPPortCheckOptions{
		Host:    "127.0.0.1",
		Ports:   []int{1},
		Timeout: 1 * time.Second,
	}, "job_002", "step_port_002")

	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].Open {
		t.Error("expected port 1 to be closed")
	}
	if len(result.Evidence) != 0 {
		t.Error("expected no evidence for closed port")
	}
}

func TestTCPPortCheckMultiple(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	openPort, _ := strconv.Atoi(portStr)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := TCPPortCheck(ctx, TCPPortCheckOptions{
		Host:    "127.0.0.1",
		Ports:   []int{openPort, 1},
		Timeout: 1 * time.Second,
	}, "job_003", "step_port_003")

	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Results))
	}
	if !result.Results[0].Open {
		t.Errorf("expected port %d to be open", openPort)
	}
	if result.Results[1].Open {
		t.Error("expected port 1 to be closed")
	}
}
