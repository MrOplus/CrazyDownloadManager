package engine

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"crazydownloadmanager/internal/model"
	"crazydownloadmanager/internal/storage"
)

// minHalf is the smallest piece dynamic segmentation will create. A busy
// segment is split only if at least 2*minHalf bytes remain in it.
const minHalf = 512 << 10

const bufSize = 64 << 10

// Progress is a snapshot of a running job.
type Progress struct {
	Size       int64
	Downloaded int64
	Active     int
	Segments   []model.Segment
}

// segJob implements IDM-style dynamic file segmentation:
//
//   - the first connection takes the whole file;
//   - every additional connection (and every connection that finishes its
//     own piece) takes the second half of the largest piece still being
//     downloaded, so no connection ever sits idle while work remains and the
//     already-open connections are reused without a new "connect" stage for
//     the existing ranges;
//   - a connection that keeps failing while others are healthy gives its
//     piece back instead of failing the download (servers that cap
//     connections per client just end up with fewer connections).
type segJob struct {
	src        Source
	file       storage.File
	ctx        context.Context
	cancel     context.CancelFunc
	resumable  bool
	maxConn    int
	timeout    time.Duration
	maxRetries int
	retryDelay time.Duration
	limiter    *rate.Limiter // per-download limit (may be Inf)
	global     *rate.Limiter // global limit (may be Inf)
	counter    *atomic.Int64 // bytes since the manager's last speed sample

	mu         sync.Mutex
	segs       []*model.Segment
	busy       map[*model.Segment]bool
	size       int64
	downloaded int64
	active     int
	err        error
	wg         sync.WaitGroup
}

func newSegJob(parent context.Context, src Source, file storage.File, size int64, resumable bool, segs []model.Segment) *segJob {
	ctx, cancel := context.WithCancel(parent)
	j := &segJob{
		src: src, file: file, ctx: ctx, cancel: cancel,
		resumable: resumable, size: size,
		busy: map[*model.Segment]bool{},
	}
	for i := range segs {
		s := segs[i]
		s.Active = false
		j.segs = append(j.segs, &s)
		j.downloaded += s.Pos - s.Start
	}
	return j
}

func (j *segJob) progress() Progress {
	j.mu.Lock()
	defer j.mu.Unlock()
	p := Progress{Size: j.size, Downloaded: j.downloaded, Active: j.active}
	p.Segments = make([]model.Segment, len(j.segs))
	for i, s := range j.segs {
		p.Segments[i] = *s
		p.Segments[i].Active = j.busy[s]
	}
	return p
}

func (j *segJob) run() error {
	n := j.maxConn
	if !j.resumable || n < 1 {
		n = 1
	}
	for i := 0; i < n; i++ {
		s := j.next()
		if s == nil {
			break
		}
		j.spawn(s)
	}
	j.wg.Wait()
	j.cancel()

	j.mu.Lock()
	defer j.mu.Unlock()
	if j.err != nil {
		return j.err
	}
	for _, s := range j.segs {
		if s.End < 0 || s.Pos < s.End {
			if err := j.ctx.Err(); err != nil {
				return err
			}
			return io.ErrUnexpectedEOF
		}
	}
	return nil
}

func (j *segJob) spawn(s *model.Segment) {
	j.wg.Add(1)
	go j.worker(s)
}

// next assigns an unclaimed unfinished segment, or splits the largest busy one.
func (j *segJob) next() *model.Segment {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.ctx.Err() != nil {
		return nil
	}
	for _, s := range j.segs {
		if !j.busy[s] && (s.End < 0 || s.Pos < s.End) {
			j.busy[s] = true
			return s
		}
	}
	if !j.resumable {
		return nil
	}
	var best *model.Segment
	var bestRem int64
	for _, s := range j.segs {
		if j.busy[s] && s.End >= 0 {
			if rem := s.End - s.Pos; rem > bestRem {
				best, bestRem = s, rem
			}
		}
	}
	if best == nil || bestRem < 2*minHalf {
		return nil
	}
	mid := best.Pos + bestRem/2
	ns := &model.Segment{Start: mid, End: best.End, Pos: mid}
	best.End = mid
	j.segs = append(j.segs, ns)
	j.busy[ns] = true
	return ns
}

func (j *segJob) release(s *model.Segment) {
	j.mu.Lock()
	delete(j.busy, s)
	j.mu.Unlock()
}

func (j *segJob) fail(err error) {
	j.mu.Lock()
	if j.err == nil {
		j.err = err
	}
	j.mu.Unlock()
	j.cancel()
}

func (j *segJob) worker(s *model.Segment) {
	defer j.wg.Done()
	j.mu.Lock()
	j.active++
	j.mu.Unlock()
	defer func() {
		j.mu.Lock()
		j.active--
		j.mu.Unlock()
	}()

	for s != nil {
		attempts := 0
		for {
			err := j.fetch(s)
			if err == nil {
				break
			}
			if j.ctx.Err() != nil {
				return
			}
			if errors.Is(err, errNoRange) {
				j.mu.Lock()
				single := len(j.segs) == 1
				if single {
					j.downloaded -= s.Pos - s.Start
					s.Pos = s.Start
					j.resumable = false
				}
				j.mu.Unlock()
				if !single {
					j.fail(fatal(errNoRange))
					return
				}
				continue
			}
			if isFatal(err) {
				j.fail(err)
				return
			}
			attempts++
			j.mu.Lock()
			others := j.active > 1
			j.mu.Unlock()
			if others && attempts >= 3 {
				// Probably a per-client connection cap: hand the piece back.
				j.release(s)
				return
			}
			if attempts > j.maxRetries {
				j.fail(err)
				return
			}
			select {
			case <-time.After(j.retryDelay):
			case <-j.ctx.Done():
				return
			}
		}
		j.release(s)
		s = j.next()
	}
}

func (j *segJob) throttle(ctx context.Context, n int) error {
	if j.limiter != nil {
		if err := j.limiter.WaitN(ctx, n); err != nil {
			return err
		}
	}
	if j.global != nil {
		return j.global.WaitN(ctx, n)
	}
	return nil
}

func (j *segJob) fetch(s *model.Segment) error {
	j.mu.Lock()
	start, end := s.Pos, s.End
	j.mu.Unlock()
	if end >= 0 && start >= end {
		return nil
	}
	ctx, cancel := context.WithCancel(j.ctx)
	defer cancel()
	var stalled atomic.Bool
	watchdog := time.AfterFunc(j.timeout, func() { stalled.Store(true); cancel() })
	defer watchdog.Stop()

	rc, err := j.src.Open(ctx, start, end)
	if err != nil {
		if stalled.Load() {
			return errStalled
		}
		return err
	}
	defer rc.Close()

	buf := make([]byte, bufSize)
	for {
		n, rerr := rc.Read(buf)
		watchdog.Reset(j.timeout)
		if n > 0 {
			j.mu.Lock()
			pos, e := s.Pos, s.End
			j.mu.Unlock()
			if e >= 0 && pos+int64(n) > e {
				n = int(e - pos) // another connection took over the rest
			}
			if n <= 0 {
				return nil
			}
			if err := j.throttle(ctx, n); err != nil {
				if stalled.Load() {
					return errStalled
				}
				return err
			}
			if _, err := j.file.WriteAt(buf[:n], pos); err != nil {
				return fatal(err)
			}
			j.mu.Lock()
			s.Pos += int64(n)
			j.downloaded += int64(n)
			done := s.End >= 0 && s.Pos >= s.End
			j.mu.Unlock()
			j.counter.Add(int64(n))
			if done {
				return nil
			}
		}
		if rerr == io.EOF {
			j.mu.Lock()
			defer j.mu.Unlock()
			if s.End < 0 {
				s.End = s.Pos
				j.size = s.Pos
				return nil
			}
			if s.Pos < s.End {
				return io.ErrUnexpectedEOF
			}
			return nil
		}
		if rerr != nil {
			if stalled.Load() {
				return errStalled
			}
			return rerr
		}
	}
}
