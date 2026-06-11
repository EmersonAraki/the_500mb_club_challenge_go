package store

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/araki/pibench/internal/cursor"
	"github.com/araki/pibench/internal/model"
	"github.com/araki/pibench/internal/resp"
)

// redisStore persists each device as a Redis sorted set keyed "t:{id}" with
// score = ts and member = the binary-encoded point. History is capped to the
// newest `cap` points per device to bound aggregate memory. The cap is enforced
// amortized: the trim command runs roughly once per trimEvery writes rather than
// on every write, since Redis is single-threaded and the write path is hot.
type redisStore struct {
	addr        string
	cap         int
	trimEvery   uint64
	dialTimeout time.Duration
	seq         atomic.Uint64
	writes      atomic.Uint64
	pool        chan *conn
}

type conn struct {
	c net.Conn
	r *bufio.Reader
	w *bufio.Writer
}

// NewRedis returns a Redis-backed Store. Connections are dialed lazily up to
// poolSize; cap bounds retained points per device.
func NewRedis(addr string, poolSize, cap int) Store {
	return &redisStore{
		addr:        addr,
		cap:         cap,
		trimEvery:   16,
		dialTimeout: 3 * time.Second,
		pool:        make(chan *conn, poolSize),
	}
}

func (s *redisStore) dial() (*conn, error) {
	c, err := net.DialTimeout("tcp", s.addr, s.dialTimeout)
	if err != nil {
		return nil, err
	}
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	return &conn{c: c, r: bufio.NewReader(c), w: bufio.NewWriter(c)}, nil
}

func (s *redisStore) get() (*conn, error) {
	select {
	case cn := <-s.pool:
		return cn, nil
	default:
		return s.dial()
	}
}

func (s *redisStore) put(cn *conn) {
	select {
	case s.pool <- cn:
	default:
		_ = cn.c.Close()
	}
}

// do runs one or more pipelined commands on a pooled connection, returning the
// reply to each. A failed connection is discarded rather than reused.
func (s *redisStore) do(ctx context.Context, cmds ...[][]byte) ([]any, error) {
	cn, err := s.get()
	if err != nil {
		return nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = cn.c.SetDeadline(dl)
	}
	replies, err := cn.roundtrip(cmds)
	if err != nil {
		_ = cn.c.Close()
		return nil, err
	}
	_ = cn.c.SetDeadline(time.Time{})
	s.put(cn)
	return replies, nil
}

func (cn *conn) roundtrip(cmds [][][]byte) ([]any, error) {
	for _, args := range cmds {
		if _, err := cn.w.Write(resp.EncodeCommand(args)); err != nil {
			return nil, err
		}
	}
	if err := cn.w.Flush(); err != nil {
		return nil, err
	}
	replies := make([]any, len(cmds))
	for i := range cmds {
		v, err := resp.ReadReply(cn.r)
		if err != nil {
			return nil, err
		}
		replies[i] = v
	}
	return replies, nil
}

// Warm eagerly establishes cap(pool) connections so the request path never pays
// a TCP handshake on a pool miss. Each connection is verified with PING, so Warm
// doubles as a readiness probe: it returns an error until Redis is reachable,
// letting the caller retry. On failure the partially filled pool is left intact
// for reuse; already-pooled connections are not discarded.
func (s *redisStore) Warm(ctx context.Context) error {
	for i := 0; i < cap(s.pool); i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		cn, err := s.dial()
		if err != nil {
			return err
		}
		if dl, ok := ctx.Deadline(); ok {
			_ = cn.c.SetDeadline(dl)
		}
		replies, err := cn.roundtrip([][][]byte{{[]byte("PING")}})
		if err != nil {
			_ = cn.c.Close()
			return err
		}
		_ = cn.c.SetDeadline(time.Time{})
		if str, ok := replies[0].(string); !ok || str != "PONG" {
			_ = cn.c.Close()
			return errors.New("redis: unexpected PING reply during warm")
		}
		s.put(cn)
	}
	return nil
}

func (s *redisStore) key(id string) []byte { return []byte("t:" + id) }

func (s *redisStore) Ping(ctx context.Context) error {
	replies, err := s.do(ctx, [][]byte{[]byte("PING")})
	if err != nil {
		return err
	}
	if str, ok := replies[0].(string); ok && str == "PONG" {
		return nil
	}
	return errors.New("redis: unexpected PING reply")
}

func (s *redisStore) Append(ctx context.Context, id string, pts []model.Point) (int, error) {
	zadd := make([][]byte, 0, 2+2*len(pts))
	zadd = append(zadd, []byte("ZADD"), s.key(id))
	zadd = encodeMembers(zadd, pts, &s.seq)

	// Amortize the cap: only pipeline the trim every trimEvery writes. Between
	// trims a device may briefly hold up to cap + trimEvery*batch extra points;
	// the next trim it lands on restores it to cap.
	cmds := [][][]byte{zadd}
	if s.writes.Add(1)%s.trimEvery == 0 {
		cmds = append(cmds, [][]byte{[]byte("ZREMRANGEBYRANK"), s.key(id),
			[]byte("0"), itoa(int64(-(s.cap + 1)))})
	}

	if _, err := s.do(ctx, cmds...); err != nil {
		return 0, err
	}
	return len(pts), nil
}

// encodeMembers appends a (score, member) byte pair to args for each point. All
// members for the batch are encoded into one shared backing array, so a batch of
// N points costs a single member allocation instead of N. seq supplies the
// per-member uniqueness counter. The returned slice is args, possibly grown.
func encodeMembers(args [][]byte, pts []model.Point, seq *atomic.Uint64) [][]byte {
	backing := make([]byte, model.EncodedLen*len(pts))
	for i, p := range pts {
		m := backing[i*model.EncodedLen : (i+1)*model.EncodedLen : (i+1)*model.EncodedLen]
		p.EncodeInto(m, seq.Add(1))
		args = append(args, itoa(p.TS), m)
	}
	return args
}

func (s *redisStore) Range(ctx context.Context, id string, from, to int64, limit int, cur string) ([]model.Point, string, error) {
	curTs, curSkip := from, 0
	if cur != "" {
		ts, skip, err := cursor.Decode(cur)
		if err != nil {
			return nil, "", err
		}
		curTs, curSkip = ts, skip
	}
	cmd := [][]byte{[]byte("ZRANGEBYSCORE"), s.key(id),
		itoa(curTs), itoa(to),
		[]byte("LIMIT"), itoa(int64(curSkip)), itoa(int64(limit + 1))}

	pts, err := s.readPoints(ctx, cmd)
	if err != nil {
		return nil, "", err
	}
	page, next := BuildPage(pts, limit, curTs, curSkip)
	return page, next, nil
}

func (s *redisStore) Recent(ctx context.Context, id string, n int) ([]model.Point, error) {
	cmd := [][]byte{[]byte("ZREVRANGE"), s.key(id), []byte("0"), itoa(int64(n - 1))}
	return s.readPoints(ctx, cmd) // ZREVRANGE is already most-recent-first
}

// readPoints runs a single command whose reply is an array of fixed-size binary
// members, decoding straight to points. It reads all members into one backing
// buffer (resp.ReadFixedBulkArray) and skips the generic []any reply path, so a
// query of N points allocates the backing + the points slice rather than ~3N
// objects. Connection handling mirrors do().
func (s *redisStore) readPoints(ctx context.Context, cmd [][]byte) ([]model.Point, error) {
	cn, err := s.get()
	if err != nil {
		return nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = cn.c.SetDeadline(dl)
	}
	if _, err := cn.w.Write(resp.EncodeCommand(cmd)); err != nil {
		_ = cn.c.Close()
		return nil, err
	}
	if err := cn.w.Flush(); err != nil {
		_ = cn.c.Close()
		return nil, err
	}
	backing, count, err := resp.ReadFixedBulkArray(cn.r, model.EncodedLen)
	if err != nil {
		_ = cn.c.Close()
		return nil, err
	}
	_ = cn.c.SetDeadline(time.Time{})
	s.put(cn)

	pts := make([]model.Point, count)
	for i := 0; i < count; i++ {
		p, err := model.Decode(backing[i*model.EncodedLen : (i+1)*model.EncodedLen])
		if err != nil {
			return nil, err
		}
		pts[i] = p
	}
	return pts, nil
}

func (s *redisStore) Close() error {
	for {
		select {
		case cn := <-s.pool:
			_ = cn.c.Close()
		default:
			return nil
		}
	}
}

// itoa renders v as decimal ASCII in a single allocation. AppendInt writes into
// the fresh backing directly, avoiding the string->[]byte copy that
// []byte(strconv.FormatInt(...)) incurs. 20 bytes holds any int64 (incl. sign).
func itoa(v int64) []byte { return strconv.AppendInt(make([]byte, 0, 20), v, 10) }
