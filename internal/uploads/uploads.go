// Package uploads streams untrusted deployment bytes into private staging.
package uploads

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
)

var ErrQuota = errors.New("upload quota exceeded")

type Result struct {
	Bytes  int64
	SHA256 [32]byte
}

// Copy copies without buffering the archive. On limit exceed, no more than one
// caller read buffer beyond the limit is consumed; callers must discard staging.
func Copy(ctx context.Context, dst io.Writer, src io.Reader, limit int64) (Result, error) {
	if limit <= 0 {
		return Result{}, ErrQuota
	}
	h := sha256.New()
	buf := make([]byte, 32*1024)
	var n int64
	for {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		default:
		}
		r, er := src.Read(buf)
		if r > 0 {
			if int64(r) > limit-n {
				return Result{}, ErrQuota
			}
			w, ew := io.MultiWriter(dst, h).Write(buf[:r])
			n += int64(w)
			if ew != nil {
				return Result{}, ew
			}
			if w != r {
				return Result{}, io.ErrShortWrite
			}
		}
		if er == io.EOF {
			var out Result
			out.Bytes = n
			copy(out.SHA256[:], h.Sum(nil))
			return out, nil
		}
		if er != nil {
			return Result{}, er
		}
	}
}
