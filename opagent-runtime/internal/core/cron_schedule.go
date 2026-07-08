package core

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func computeScheduledNextRun(schedule CronTaskSchedule, now time.Time) (time.Time, error) {
	now = now.In(time.Local)
	switch {
	case strings.TrimSpace(schedule.Every) != "":
		duration, err := time.ParseDuration(strings.TrimSpace(schedule.Every))
		if err != nil {
			return time.Time{}, err
		}
		return now.Add(duration), nil
	case strings.TrimSpace(schedule.Time) != "":
		return computeNextDailyRun(strings.TrimSpace(schedule.Time), now)
	case strings.TrimSpace(schedule.Cron) != "":
		return computeNextCronRun(strings.TrimSpace(schedule.Cron), now)
	default:
		return time.Time{}, fmt.Errorf("schedule is empty")
	}
}

func advanceScheduledNextRun(schedule CronTaskSchedule, previousScheduledAt, now time.Time) (time.Time, error) {
	now = now.In(time.Local)
	previousScheduledAt = previousScheduledAt.In(time.Local)
	switch {
	case strings.TrimSpace(schedule.Every) != "":
		duration, err := time.ParseDuration(strings.TrimSpace(schedule.Every))
		if err != nil {
			return time.Time{}, err
		}
		next := previousScheduledAt.Add(duration)
		for !next.After(now) {
			next = next.Add(duration)
		}
		return next, nil
	case strings.TrimSpace(schedule.Time) != "":
		return computeNextDailyRun(strings.TrimSpace(schedule.Time), now)
	case strings.TrimSpace(schedule.Cron) != "":
		return computeNextCronRun(strings.TrimSpace(schedule.Cron), now)
	default:
		return time.Time{}, fmt.Errorf("schedule is empty")
	}
}

func validateCronTaskSchedule(schedule CronTaskSchedule) error {
	count := 0
	if strings.TrimSpace(schedule.Cron) != "" {
		count++
	}
	if strings.TrimSpace(schedule.Every) != "" {
		count++
	}
	if strings.TrimSpace(schedule.Time) != "" {
		count++
	}
	switch count {
	case 0:
		return errors.New("schedule requires exactly one of cron, every, or time")
	case 1:
		// valid
	default:
		return errors.New("schedule fields cron, every, and time are mutually exclusive")
	}
	if raw := strings.TrimSpace(schedule.Every); raw != "" {
		if _, err := time.ParseDuration(raw); err != nil {
			return fmt.Errorf("schedule.every must be a valid duration: %w", err)
		}
	}
	if raw := strings.TrimSpace(schedule.Cron); raw != "" {
		if len(strings.Fields(raw)) != 5 {
			return fmt.Errorf("schedule.cron must use 5 fields, got %d", len(strings.Fields(raw)))
		}
	}
	if raw := strings.TrimSpace(schedule.Time); raw != "" {
		if _, err := parseScheduleClock(raw); err != nil {
			return fmt.Errorf("schedule.time must use HH:MM or HH:MM:SS: %w", err)
		}
	}
	return nil
}

func parseScheduleClock(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("empty time")
	}
	for _, layout := range []string{"15:04", "15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q", value)
}

func computeNextDailyRun(clock string, now time.Time) (time.Time, error) {
	clock = strings.TrimSpace(clock)
	parsed, err := time.Parse("15:04", clock)
	if err != nil {
		parsed, err = time.Parse("15:04:05", clock)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid time %q", clock)
		}
	}
	candidate := time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), parsed.Second(), 0, now.Location())
	if !candidate.After(now) {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate, nil
}

func computeNextCronRun(expr string, now time.Time) (time.Time, error) {
	schedule, err := cronParser.Parse(expr)
	if err != nil {
		return time.Time{}, err
	}
	next := schedule.Next(now)
	if next.IsZero() {
		return time.Time{}, fmt.Errorf("cron expression %q did not produce a next run", expr)
	}
	return next.In(time.Local), nil
}

func cronBackoffDelay(consecutiveErrors int) time.Duration {
	switch {
	case consecutiveErrors <= 1:
		return 30 * time.Second
	case consecutiveErrors == 2:
		return time.Minute
	default:
		return 5 * time.Minute
	}
}
