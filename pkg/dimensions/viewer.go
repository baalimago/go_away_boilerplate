package dimensions

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
)

// Option configures a Viewer. The zero value of every option selects the
// production behavior: a real ioctl reader and a process-wide SIGWINCH
// registration. Tests inject readers and signal sources to stay deterministic
// and to avoid sending process-global signals.
type Option func(*viewerOptions)

type viewerOptions struct {
	reader       Reader
	signalSource <-chan os.Signal
}

// WithReader replaces the ioctl-based reader with r. A nil reader selects the
// default ioctl reader. The reader must be safe to call many times; the
// Viewer serializes all calls.
func WithReader(r Reader) Option {
	return func(o *viewerOptions) { o.reader = r }
}

// WithSignalSource replaces the process-wide SIGWINCH registration with src.
// The Viewer selects on src; when src is closed, the Viewer stops exactly as
// if Stop had been called. A nil source selects the default process-wide
// SIGWINCH registration, which the Viewer owns and releases on stop.
func WithSignalSource(src <-chan os.Signal) Option {
	return func(o *viewerOptions) { o.signalSource = src }
}

// Viewer observes the size of one terminal file descriptor. It performs an
// initial read, keeps the last valid size, and watches for SIGWINCH so a
// resize produces a fresh snapshot on Events.
//
// The Viewer borrows the file descriptor: it never closes it. The descriptor
// must be the file the caller actually writes to, so the observed size
// matches the output target.
//
// A Viewer is safe for concurrent use. Snapshot and Events may be called from
// any goroutine; the resize watcher runs in its own goroutine started by New.
type Viewer struct {
	read Reader
	sig  <-chan os.Signal
	own  chan os.Signal // non-nil when the Viewer registered signal.Notify itself

	mu           sync.Mutex
	current      Dimensions
	valid        bool
	eventsClosed bool

	events chan Dimensions
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
}

// New creates a Viewer bound to fd, performs the initial read, and starts a
// watcher that refreshes the size on SIGWINCH. The watcher runs until the
// context is done, the signal source is closed, or Stop is called.
//
// A failed initial read is not an error: the failure surfaces through
// Snapshot. ctx must not be nil.
func New(ctx context.Context, fd uintptr, opts ...Option) *Viewer {
	opt := viewerOptions{}
	for _, apply := range opts {
		apply(&opt)
	}
	read := opt.reader
	if read == nil {
		read = DefaultReader(fd)
	}
	sig := opt.signalSource
	var own chan os.Signal
	if sig == nil {
		own = make(chan os.Signal, 1)
		registerSigwinch(own)
		sig = own
	}
	v := &Viewer{
		read:   read,
		sig:    sig,
		own:    own,
		events: make(chan Dimensions, 1),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	v.mu.Lock()
	d, err := normalizeRead(read())
	if err == nil {
		v.current = d
		v.valid = true
	}
	v.mu.Unlock()
	go v.watch(ctx)
	return v
}

// Snapshot performs one live read and returns the size together with the read
// result. On success the returned size becomes the last valid snapshot. On
// failure Snapshot returns the last valid snapshot, or Fallback when no valid
// snapshot exists yet, and wraps the read error so errors.Is(err,
// ErrUnavailable) holds. Snapshot remains usable after the Viewer has stopped.
func (v *Viewer) Snapshot() (Dimensions, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, err := normalizeRead(v.read())
	if err == nil {
		v.current = d
		v.valid = true
		return d, nil
	}
	if v.valid {
		return v.current, fmt.Errorf("read terminal dimensions: %w", err)
	}
	return Fallback, fmt.Errorf("read terminal dimensions: %w", err)
}

// Events returns the Viewer's resize channel. It delivers a fresh snapshot
// after every successful refresh triggered by a signal. The channel has
// capacity one: when the consumer is slower than a burst of resizes,
// intermediate snapshots are dropped and only the newest buffered snapshot
// survives; Snapshot always returns the latest state.
//
// The channel is closed when the Viewer stops, so receivers must use the
// two-value receive form or range over the channel:
//
//	for d := range v.Events() { ... }
//
// or
//
//	select {
//	case d, ok := <-v.Events():
//		if !ok { return }
//		...
//	}
func (v *Viewer) Events() <-chan Dimensions {
	return v.events
}

// Stop ends the watcher, releases the process-wide signal registration when
// the Viewer owns one, and closes Events. Stop blocks until the watcher has
// exited and is idempotent: calling it again, after context cancellation, or
// after the signal source closed returns immediately.
func (v *Viewer) Stop() {
	v.once.Do(func() { close(v.stop) })
	<-v.done
}

func (v *Viewer) watch(ctx context.Context) {
	defer v.finish()
	for {
		select {
		case <-ctx.Done():
			return
		case <-v.stop:
			return
		case _, ok := <-v.sig:
			if !ok {
				return
			}
			v.refresh()
		}
	}
}

// refresh reads the size and, on success, updates the last valid snapshot and
// delivers the fresh snapshot on Events without blocking. Nothing is written
// to the terminal and no render state is touched, so refresh is safe to run
// from the signal path.
func (v *Viewer) refresh() {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, err := normalizeRead(v.read())
	if err != nil || v.eventsClosed {
		return
	}
	v.current = d
	v.valid = true
	select {
	case v.events <- d:
	default:
	}
}

// normalizeRead turns a raw read result into a usable size or an
// ErrUnavailable-wrapped failure. A successful read that reports zero width
// or height is an unusable size, so every Reader observes the same zero-size
// policy as the default ioctl reader.
func normalizeRead(d Dimensions, err error) (Dimensions, error) {
	if err != nil {
		return d, err
	}
	if d.Width <= 0 || d.Height <= 0 {
		return d, fmt.Errorf("terminal reports unusable size %dx%d: %w", d.Width, d.Height, ErrUnavailable)
	}
	return d, nil
}

// finish releases resources exactly once: it stops the owned signal
// registration, closes Events, and signals that the watcher has exited.
func (v *Viewer) finish() {
	v.mu.Lock()
	if v.own != nil {
		signal.Stop(v.own)
		v.own = nil
	}
	if !v.eventsClosed {
		close(v.events)
		v.eventsClosed = true
	}
	v.mu.Unlock()
	close(v.done)
}
