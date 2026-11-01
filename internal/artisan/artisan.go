package artisan

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type Runner struct{ Root, PHP string }

var idRE = regexp.MustCompile(`^(?:[0-9]{1,20}|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)

func (r Runner) Run(ctx context.Context, args ...string) (string, error) {
	cmd := r.command(ctx, args...)
	b, e := cmd.CombinedOutput()
	if len(b) > 4000 {
		b = b[:4000]
	}
	if e != nil && (errors.Is(e, exec.ErrNotFound) || errors.Is(e, fs.ErrNotExist) || strings.Contains(e.Error(), "executable file not found")) {
		return string(b), errors.New("php not found; set LARASCOPE_PHP")
	}
	return string(b), e
}

func (r Runner) command(ctx context.Context, args ...string) *exec.Cmd {
	p := r.PHP
	if strings.TrimSpace(p) == "" {
		p = os.Getenv("LARASCOPE_PHP")
	}
	if strings.TrimSpace(p) == "" {
		p = "php"
	}
	x := strings.Fields(p)
	if len(x) == 0 {
		x = []string{"php"}
	}
	cmd := exec.CommandContext(ctx, x[0], append(x[1:], append([]string{"artisan"}, args...)...)...)
	cmd.Dir = r.Root
	return cmd
}
func (r Runner) Retry(c context.Context, ids ...string) (string, error) {
	if len(ids) == 0 {
		return "", errors.New("invalid job id")
	}
	for _, id := range ids {
		if !idRE.MatchString(id) {
			return "", errors.New("invalid job id")
		}
	}
	return r.Run(c, append([]string{"queue:retry"}, ids...)...)
}
func (r Runner) RetryAll(c context.Context) (string, error) { return r.Run(c, "queue:retry", "all") }
func (r Runner) Forget(c context.Context, id string) (string, error) {
	if !idRE.MatchString(id) {
		return "", errors.New("invalid job id")
	}
	return r.Run(c, "queue:forget", id)
}
func (r Runner) Flush(c context.Context) (string, error) { return r.Run(c, "queue:flush") }
