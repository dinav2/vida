// Package ipc implements Unix socket IPC for vida using newline-delimited JSON.
package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

// HistoryEntry represents one turn in a multi-turn AI conversation (FR-04b).
type HistoryEntry struct {
	Role    string `json:"role"`    // "user" | "assistant"
	Content string `json:"content"`
}

// Message is a JSON message sent over the IPC socket.
type Message struct {
	Type    string `json:"type"`
	Raw     []byte `json:"-"` // raw JSON bytes of a received message (no trailing newline)
	Message string `json:"message,omitempty"`
	ID      string `json:"id,omitempty"`
	Input   string `json:"input,omitempty"`

	// Extended fields used by specific response types.
	PID      int    `json:"pid,omitempty"`
	Provider string `json:"provider,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Value    string `json:"value,omitempty"`
	URL      string `json:"url,omitempty"`
	IDs      string `json:"ids,omitempty"`   // newline-separated desktop IDs for app_list
	Exec     string `json:"exec,omitempty"`  // newline-separated exec strings for app_list
	Icons    string `json:"icons"` // newline-separated icon names for app_list (always sent in app_list)

	// Command mode fields.
	Name    string         `json:"name,omitempty"`    // command name for run_command
	History []HistoryEntry `json:"history,omitempty"` // multi-turn conversation history
}

// ReplyFunc sends a reply message to the connected client.
type ReplyFunc func(Message) error

// --- Server ---

// syncEnc is a json.Encoder protected by a mutex so concurrent goroutines
// (e.g. handler reply and broadcast) don't interleave bytes on the same conn.
type syncEnc struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func (e *syncEnc) encode(v any) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.enc.Encode(v)
}

// Server is a Unix socket IPC server.
type Server struct {
	l       net.Listener
	handler func(Message, ReplyFunc)

	mu   sync.Mutex
	subs map[net.Conn]*syncEnc // subscriber connections
}

// Listen creates a Unix socket server at path.
// Stale socket files are silently removed (FR-01b).
// Socket permissions are set to 0600 (TR-02e).
func Listen(path string) (*Server, error) {
	_ = os.Remove(path) // remove stale socket file; ignore error if absent
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("ipc listen: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		l.Close()
		return nil, fmt.Errorf("ipc chmod: %w", err)
	}
	return &Server{
		l:    l,
		subs: make(map[net.Conn]*syncEnc),
	}, nil
}

// Handle registers a handler called for every received message.
// The handler may call reply to send an additional response.
// Built-in handling (ping, subscribe, show, hide) always runs after the handler.
func (s *Server) Handle(h func(Message, ReplyFunc)) {
	s.handler = h
}

// Serve accepts connections until the listener is closed.
func (s *Server) Serve(ctx context.Context) {
	for {
		conn, err := s.l.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

// Close shuts down the listener.
func (s *Server) Close() error {
	return s.l.Close()
}

func (s *Server) handleConn(conn net.Conn) {
	defer func() {
		conn.Close()
		s.mu.Lock()
		delete(s.subs, conn)
		s.mu.Unlock()
	}()

	se := &syncEnc{enc: json.NewEncoder(conn)}
	reply := func(msg Message) error { return se.encode(msg) }

	// Use a scanner so each line (one JSON object) is read atomically.
	// Bytes() returns the line WITHOUT the trailing newline — exactly what
	// msg.Raw needs (FR-03a).
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		// Copy: scanner reuses its internal buffer on the next Scan call.
		rawCopy := make([]byte, len(raw))
		copy(rawCopy, raw)

		var msg Message
		if err := json.Unmarshal(rawCopy, &msg); err != nil {
			_ = reply(Message{Type: "error", Message: "invalid JSON"})
			continue
		}
		msg.Raw = rawCopy

		if s.handler != nil {
			s.handler(msg, reply)
		}

		s.dispatch(msg, conn, se, reply)
	}
}

// dispatch applies built-in message handling for ping, subscribe, show, hide.
// When no application handler is registered, unknown types receive an error
// reply. When a handler is registered, it is authoritative for non-built-in
// types and dispatch stays silent (avoids double-replies).
func (s *Server) dispatch(msg Message, conn net.Conn, se *syncEnc, reply func(Message) error) {
	switch msg.Type {
	case "ping":
		_ = reply(Message{Type: "pong"})
	case "subscribe":
		s.mu.Lock()
		s.subs[conn] = se
		s.mu.Unlock()
		// no reply: subscriber uses SendNoReply
	case "show", "hide":
		s.broadcast(Message{Type: msg.Type})
		_ = reply(Message{Type: "ok"})
	default:
		if s.handler == nil {
			_ = reply(Message{Type: "error", Message: "unknown type"})
		}
	}
}

// broadcast sends msg to all subscribed connections.
// Dead subscribers are pruned.
func (s *Server) broadcast(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var dead []net.Conn
	for conn, se := range s.subs {
		if err := se.encode(msg); err != nil {
			dead = append(dead, conn)
		}
	}
	for _, c := range dead {
		delete(s.subs, c)
		c.Close()
	}
}

// --- One-shot client ---

// DaemonNotRunningError is returned when the daemon socket cannot be reached.
type DaemonNotRunningError struct {
	cause error
}

func (e *DaemonNotRunningError) Error() string {
	return "vida daemon is not running. Start it with: vida-daemon &"
}

func (e *DaemonNotRunningError) Unwrap() error { return e.cause }

// IsDaemonNotRunning reports whether err means the daemon is not running.
func IsDaemonNotRunning(err error) bool {
	var e *DaemonNotRunningError
	return errors.As(err, &e)
}

// Conn is a one-shot client: send one message, receive one reply, close.
type Conn struct {
	conn net.Conn
	enc  *json.Encoder
	dec  *json.Decoder
}

// Connect opens a one-shot connection to the daemon socket (SCN-20).
func Connect(path string) (*Conn, error) {
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return nil, &DaemonNotRunningError{cause: err}
	}
	return &Conn{
		conn: conn,
		enc:  json.NewEncoder(conn),
		dec:  json.NewDecoder(conn),
	}, nil
}

// Send encodes msg, writes it, and decodes the single response.
func (c *Conn) Send(msg Message) (Message, error) {
	if err := c.enc.Encode(msg); err != nil {
		return Message{}, err
	}
	var resp Message
	if err := c.dec.Decode(&resp); err != nil {
		return Message{}, err
	}
	return resp, nil
}

// Close closes the connection.
func (c *Conn) Close() error { return c.conn.Close() }

// --- Persistent client ---

// PersistentConn is a long-lived client that can receive unsolicited messages
// (e.g. show/hide broadcasts from the daemon).
type PersistentConn struct {
	conn      net.Conn
	mu        sync.Mutex
	enc       *json.Encoder
	incoming  chan Message
	done      chan struct{}
	closeOnce sync.Once
}

// ConnectPersistent opens a persistent connection to the daemon socket.
func ConnectPersistent(path string) (*PersistentConn, error) {
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return nil, &DaemonNotRunningError{cause: err}
	}
	c := &PersistentConn{
		conn:     conn,
		enc:      json.NewEncoder(conn),
		incoming: make(chan Message, 16),
		done:     make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

func (c *PersistentConn) readLoop() {
	defer close(c.incoming)
	dec := json.NewDecoder(c.conn)
	for {
		var msg Message
		if err := dec.Decode(&msg); err != nil {
			return
		}
		select {
		case c.incoming <- msg:
		case <-c.done:
			return
		}
	}
}

// SendNoReply sends msg without waiting for a response.
func (c *PersistentConn) SendNoReply(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enc.Encode(msg)
}

// Recv waits up to timeout for the next incoming message.
func (c *PersistentConn) Recv(timeout time.Duration) (Message, error) {
	select {
	case msg, ok := <-c.incoming:
		if !ok {
			return Message{}, errors.New("connection closed")
		}
		return msg, nil
	case <-time.After(timeout):
		return Message{}, fmt.Errorf("recv timeout after %v", timeout)
	}
}

// Close closes the persistent connection.
func (c *PersistentConn) Close() error {
	c.closeOnce.Do(func() { close(c.done) })
	return c.conn.Close()
}
