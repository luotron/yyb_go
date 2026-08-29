package scriptrunner

import (
	"testing"
	"time"
)

func TestParseCronValid(t *testing.T) {
	for _, raw := range []string{
		"* * * * *",
		"32 16 * * *",
		"0 0 1 1 *",
		"*/5 * * * *",
		"0 9-18 * * 1-5",
		"0,30 * * * *",
		"0 8 1,15 * *",
		"? ? * * ?",
	} {
		if _, err := parseCron(raw); err != nil {
			t.Errorf("parseCron(%q) error = %v", raw, err)
		}
	}
}

func TestParseCronInvalid(t *testing.T) {
	for _, raw := range []string{
		"",
		"* * * *",
		"* * * * * *",
		"60 * * * *",
		"* 24 * * *",
		"* * 0 * *",
		"* * 32 * *",
		"* * * 13 *",
		"* * * * 7",
		"a * * * *",
		"*/0 * * * *",
		"1- * * * *",
		", * * * *",
	} {
		if _, err := parseCron(raw); err == nil {
			t.Errorf("parseCron(%q) 应当失败", raw)
		}
	}
}

func TestScheduleNext(t *testing.T) {
	schedule, err := parseCron("32 16 * * *")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 29, 10, 0, 0, 0, time.Local)
	next := schedule.next(base)
	want := time.Date(2026, 8, 29, 16, 32, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("next = %v, want %v", next, want)
	}
	// 当天 16:32 之后应落到次日。
	late := time.Date(2026, 8, 29, 17, 0, 0, 0, time.Local)
	next = schedule.next(late)
	want = time.Date(2026, 8, 30, 16, 32, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("next after same day = %v, want %v", next, want)
	}
}

func TestScheduleNextStepAndRanges(t *testing.T) {
	schedule, err := parseCron("*/15 9-10 * * *")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 29, 8, 59, 0, 0, time.Local)
	next := schedule.next(base)
	want := time.Date(2026, 8, 29, 9, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("next = %v, want %v", next, want)
	}
	next = schedule.next(time.Date(2026, 8, 29, 10, 45, 0, 0, time.Local))
	want = time.Date(2026, 8, 30, 9, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("next after range = %v, want %v", next, want)
	}
}

func TestScheduleWeekdayVsDayOr(t *testing.T) {
	// 只限制星期：2026-08-29 是周六，下个周一为 08-31。
	schedule, err := parseCron("0 8 * * 1")
	if err != nil {
		t.Fatal(err)
	}
	next := schedule.next(time.Date(2026, 8, 29, 9, 0, 0, 0, time.Local))
	want := time.Date(2026, 8, 31, 8, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("next = %v, want %v", next, want)
	}

	// 日与周同时受限按“或”匹配：周六 08-29 同时命中星期，直接当天触发。
	schedule, err = parseCron("0 8 29 * 6")
	if err != nil {
		t.Fatal(err)
	}
	next = schedule.next(time.Date(2026, 8, 28, 9, 0, 0, 0, time.Local))
	want = time.Date(2026, 8, 29, 8, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("day-or-weekday next = %v, want %v", next, want)
	}
}

func TestValidName(t *testing.T) {
	for _, name := range []string{"demo.py", "my_script-1.py", "A.py", "签到.py"} {
		if !ValidName(name) {
			t.Errorf("ValidName(%q) = false", name)
		}
	}
	for _, name := range []string{"", "demo", "demo.txt", "../x.py", "a/b.py", `a\b.py`, "-x.py", ".py"} {
		if ValidName(name) {
			t.Errorf("ValidName(%q) = true", name)
		}
	}
}
