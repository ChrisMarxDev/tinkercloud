package update

import (
	"context"
	"os"
	"os/exec"
	"time"
)

type ExecHealth struct {
	Binary, Config string
	Timeout        time.Duration
	Env            []string
}

func (h ExecHealth) Check(ctx context.Context) error {
	st, e := os.Lstat(h.Binary)
	if e != nil || st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
		return ErrHealth
	}
	to := h.Timeout
	if to <= 0 {
		to = 10 * time.Second
	}
	c, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	// A candidate must run the full doctor suite while its updater-owned
	// rollback snapshot remains available. The hidden mode is intentionally
	// scoped to this health check; ordinary doctor/status still report a
	// residual snapshot as degraded.
	cmd := exec.CommandContext(c, h.Binary, "doctor", "--config", h.Config, "--update-health-check")
	cmd.Env = h.Env
	out, e := cmd.Output()
	if e != nil || c.Err() != nil || len(out) > 64<<10 {
		return ErrHealth
	}
	return nil
}
