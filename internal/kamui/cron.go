package kamui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cron is a parsed cron expression: 5 fields (min hour dom mon dow) or 6 with
// leading seconds. Fields take *, a, a-b, */n, a-b/n and comma lists.
type Cron struct {
	sec, min, hour, dom, mon, dow [64]bool
	domStar, dowStar              bool
}

// ParseCron parses expr.
func ParseCron(expr string) (*Cron, error) {
	f := strings.Fields(expr)
	if len(f) == 5 {
		f = append([]string{"0"}, f...)
	}
	if len(f) != 6 {
		return nil, fmt.Errorf("%q: want 5 or 6 fields", expr)
	}
	c := &Cron{domStar: f[3] == "*", dowStar: f[5] == "*"}
	specs := []struct {
		dst    *[64]bool
		lo, hi int
		name   string
	}{{&c.sec, 0, 59, "second"}, {&c.min, 0, 59, "minute"}, {&c.hour, 0, 23, "hour"},
		{&c.dom, 1, 31, "day of month"}, {&c.mon, 1, 12, "month"}, {&c.dow, 0, 7, "day of week"}}
	for i, s := range specs {
		if err := parseField(f[i], s.lo, s.hi, s.dst); err != nil {
			return nil, fmt.Errorf("%q: %s: %w", expr, s.name, err)
		}
	}
	if c.dow[7] {
		c.dow[0] = true // 7 is Sunday too
	}
	return c, nil
}

func parseField(f string, lo, hi int, dst *[64]bool) error {
	for _, part := range strings.Split(f, ",") {
		step := 1
		if a, b, ok := strings.Cut(part, "/"); ok {
			n, err := strconv.Atoi(b)
			if err != nil || n <= 0 {
				return fmt.Errorf("bad step %q", b)
			}
			step, part = n, a
		}
		from, to := lo, hi
		if part != "*" {
			a, b, isRange := strings.Cut(part, "-")
			x, err := strconv.Atoi(a)
			if err != nil {
				return fmt.Errorf("bad value %q", a)
			}
			from, to = x, x
			if isRange {
				y, err := strconv.Atoi(b)
				if err != nil {
					return fmt.Errorf("bad value %q", b)
				}
				to = y
			} else if step > 1 {
				to = hi
			}
		}
		if from < lo || to > hi || from > to {
			return fmt.Errorf("%d-%d out of range %d-%d", from, to, lo, hi)
		}
		for v := from; v <= to; v += step {
			dst[v] = true
		}
	}
	return nil
}

func (c *Cron) dayMatches(t time.Time) bool {
	d, w := c.dom[t.Day()], c.dow[int(t.Weekday())]
	switch {
	case c.domStar && c.dowStar:
		return true
	case c.domStar:
		return w
	case c.dowStar:
		return d
	}
	return d || w // standard cron: both restricted means either
}

// Next returns the first matching time strictly after t (local time), or the
// zero time if nothing matches within a year.
func (c *Cron) Next(t time.Time) time.Time {
	t = t.Truncate(time.Second).Add(time.Second)
	end := t.AddDate(1, 0, 1)
	for t.Before(end) {
		if !c.mon[int(t.Month())] || !c.dayMatches(t) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !c.hour[t.Hour()] {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}
		if !c.min[t.Minute()] {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute()+1, 0, 0, t.Location())
			continue
		}
		for s := t.Second(); s < 60; s++ {
			if c.sec[s] {
				return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), s, 0, t.Location())
			}
		}
		t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute()+1, 0, 0, t.Location())
	}
	return time.Time{}
}
