package scriptrunner

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule 表示 5 段 cron 表达式（分 时 日 月 周），周 0-6 对应周日到周六。
type Schedule struct {
	raw      string
	minutes  cronField
	hours    cronField
	days     cronField
	months   cronField
	weekdays cronField
}

type cronStep struct {
	start int
	end   int
	step  int
}

type cronField struct {
	min   int
	max   int
	steps []cronStep
}

func (s Schedule) Raw() string { return s.raw }

func parseCron(raw string) (Schedule, error) {
	parts := strings.Fields(raw)
	if len(parts) != 5 {
		return Schedule{}, fmt.Errorf("cron 必须是 5 段（分 时 日 月 周），例如 %q", "32 16 * * *")
	}
	var (
		s   Schedule
		err error
	)
	if s.minutes, err = parseCronField(parts[0], 0, 59); err != nil {
		return Schedule{}, fmt.Errorf("分钟段 %q 无效: %w", parts[0], err)
	}
	if s.hours, err = parseCronField(parts[1], 0, 23); err != nil {
		return Schedule{}, fmt.Errorf("小时段 %q 无效: %w", parts[1], err)
	}
	if s.days, err = parseCronField(parts[2], 1, 31); err != nil {
		return Schedule{}, fmt.Errorf("日期段 %q 无效: %w", parts[2], err)
	}
	if s.months, err = parseCronField(parts[3], 1, 12); err != nil {
		return Schedule{}, fmt.Errorf("月份段 %q 无效: %w", parts[3], err)
	}
	if s.weekdays, err = parseCronField(parts[4], 0, 6); err != nil {
		return Schedule{}, fmt.Errorf("星期段 %q 无效: %w", parts[4], err)
	}
	s.raw = strings.Join(parts, " ")
	return s, nil
}

func parseCronField(raw string, min, max int) (cronField, error) {
	field := cronField{min: min, max: max}
	for _, part := range strings.Split(raw, ",") {
		if part == "" {
			return cronField{}, fmt.Errorf("存在空片段")
		}
		base, step := part, 1
		if idx := strings.IndexByte(part, '/'); idx >= 0 {
			base = part[:idx]
			parsed, err := strconv.Atoi(part[idx+1:])
			if err != nil || parsed <= 0 {
				return cronField{}, fmt.Errorf("步长 %q 无效", part[idx+1:])
			}
			step = parsed
		}
		start, end := min, max
		switch {
		case base == "*" || base == "?":
		case strings.Contains(base, "-"):
			bounds := strings.SplitN(base, "-", 2)
			lo, err1 := strconv.Atoi(bounds[0])
			hi, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil {
				return cronField{}, fmt.Errorf("范围 %q 无效", base)
			}
			start, end = lo, hi
		default:
			value, err := strconv.Atoi(base)
			if err != nil {
				return cronField{}, fmt.Errorf("数值 %q 无效", base)
			}
			start, end = value, value
		}
		if start < min || end > max || start > end {
			return cronField{}, fmt.Errorf("数值 %d-%d 超出允许范围 %d-%d", start, end, min, max)
		}
		field.steps = append(field.steps, cronStep{start: start, end: end, step: step})
	}
	return field, nil
}

func (f cronField) match(value int) bool {
	for _, step := range f.steps {
		if value >= step.start && value <= step.end && (value-step.start)%step.step == 0 {
			return true
		}
	}
	return false
}

func (f cronField) restricted() bool {
	if len(f.steps) != 1 {
		return true
	}
	step := f.steps[0]
	return step.start != f.min || step.end != f.max || step.step != 1
}

// next 返回 after 之后的第一个触发时间；一年内无匹配返回零值。
func (s Schedule) next(after time.Time) time.Time {
	limit := after.Add(366 * 24 * time.Hour)
	t := after.Truncate(time.Minute).Add(time.Minute)
	for !t.After(limit) {
		if s.months.match(int(t.Month())) && s.matchDay(t) &&
			s.hours.match(t.Hour()) && s.minutes.match(t.Minute()) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
}

func (s Schedule) matchDay(t time.Time) bool {
	dayOK := s.days.match(t.Day())
	weekdayOK := s.weekdays.match(int(t.Weekday()))
	// Vixie cron 语义：日与周都受限时按“或”匹配，否则两者都需满足。
	if s.days.restricted() && s.weekdays.restricted() {
		return dayOK || weekdayOK
	}
	return dayOK && weekdayOK
}
