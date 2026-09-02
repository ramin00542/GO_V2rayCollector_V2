package proxy

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"time"
)

// pastDeadline is a deadline in the past used to unblock a pending operation
// when the context is cancelled.
var pastDeadline = time.Unix(1, 0)

// withContextIO runs fn while honouring ctx cancellation. Proxy handshakes must
// never outlive their context, otherwise a stuck server would hold a config
// test open forever.
func withContextIO(ctx context.Context, conn net.Conn, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
		defer func() { _ = conn.SetDeadline(time.Time{}) }()
	}

	done := make(chan error, 1)
	go func() { done <- fn() }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// Unblock the pending operation, then wait for it to return so the
		// connection is not used by two goroutines at once.
		_ = conn.SetDeadline(pastDeadline)
		<-done
		_ = conn.SetDeadline(time.Time{})
		return ctx.Err()
	}
}

func writeWithContext(ctx context.Context, conn net.Conn, payload []byte) error {
	return withContextIO(ctx, conn, func() error {
		_, err := conn.Write(payload)
		return err
	})
}

func readFullWithContext(ctx context.Context, conn net.Conn, buffer []byte) (int, error) {
	var n int
	err := withContextIO(ctx, conn, func() error {
		var err error
		n, err = io.ReadFull(conn, buffer)
		return err
	})
	return n, err
}

// readLineWithContext reads a single CRLF terminated line without the line break.
func readLineWithContext(ctx context.Context, conn net.Conn, reader *bufio.Reader) (string, error) {
	var line string
	err := withContextIO(ctx, conn, func() error {
		var err error
		line, err = reader.ReadString('\n')
		if err != nil && err != io.EOF && len(line) > 0 {
			// Tolerate a final line without a terminator.
			err = nil
		}
		return err
	})
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
