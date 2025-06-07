package node

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

type testCaseCronSpecField struct {
	name   string
	in     string
	field  cronField
	out    cronMaskList
	outerr error
}
type testCaseCronField struct {
	spec string
	out  []time.Time
}

func TestCronParse1(t *testing.T) {
	timeParse := func(s string) time.Time {
		t, _ := time.Parse(time.RFC3339, s)
		return t
	}

	// can be also validated using https://cronjob.xyz/
	// all the other services i found have incomplete cronspec support.
	// as an example:
	//    1 19 * * 1#1,7L
	//    run at 19:01 every month on the first monday and last sunday

	cases := []testCaseCronField{
		{"1 19 * * 3#2",
			[]time.Time{
				timeParse("2025-01-08T19:01:00Z"),
				timeParse("2025-02-12T19:01:00Z"),
				timeParse("2025-03-12T19:01:00Z"),
				timeParse("2025-04-09T19:01:00Z"),
				timeParse("2025-05-14T19:01:00Z"),
				timeParse("2025-06-11T19:01:00Z"),
				timeParse("2025-07-09T19:01:00Z"),
				timeParse("2025-08-13T19:01:00Z"),
				timeParse("2025-09-10T19:01:00Z"),
				timeParse("2025-10-08T19:01:00Z"),
				timeParse("2025-11-12T19:01:00Z"),
				timeParse("2025-12-10T19:01:00Z"),
			},
		},
		{"1 19 3 * 3#2",
			[]time.Time{
				timeParse("2025-01-03T19:01:00Z"),
				timeParse("2025-01-08T19:01:00Z"),
				timeParse("2025-02-03T19:01:00Z"),
				timeParse("2025-02-12T19:01:00Z"),
				timeParse("2025-03-03T19:01:00Z"),
				timeParse("2025-03-12T19:01:00Z"),
				timeParse("2025-04-03T19:01:00Z"),
				timeParse("2025-04-09T19:01:00Z"),
				timeParse("2025-05-03T19:01:00Z"),
				timeParse("2025-05-14T19:01:00Z"),
				timeParse("2025-06-03T19:01:00Z"),
				timeParse("2025-06-11T19:01:00Z"),
				timeParse("2025-07-03T19:01:00Z"),
				timeParse("2025-07-09T19:01:00Z"),
				timeParse("2025-08-03T19:01:00Z"),
				timeParse("2025-08-13T19:01:00Z"),
				timeParse("2025-09-03T19:01:00Z"),
				timeParse("2025-09-10T19:01:00Z"),
				timeParse("2025-10-03T19:01:00Z"),
				timeParse("2025-10-08T19:01:00Z"),
				timeParse("2025-11-03T19:01:00Z"),
				timeParse("2025-11-12T19:01:00Z"),
				timeParse("2025-12-03T19:01:00Z"),
				timeParse("2025-12-10T19:01:00Z"),
			},
		},
		{"1 19 3 * *",
			[]time.Time{
				timeParse("2025-01-03T19:01:00Z"),
				timeParse("2025-02-03T19:01:00Z"),
				timeParse("2025-03-03T19:01:00Z"),
				timeParse("2025-04-03T19:01:00Z"),
				timeParse("2025-05-03T19:01:00Z"),
				timeParse("2025-06-03T19:01:00Z"),
				timeParse("2025-07-03T19:01:00Z"),
				timeParse("2025-08-03T19:01:00Z"),
				timeParse("2025-09-03T19:01:00Z"),
				timeParse("2025-10-03T19:01:00Z"),
				timeParse("2025-11-03T19:01:00Z"),
				timeParse("2025-12-03T19:01:00Z"),
			},
		},
		{"1 19 */15,L 2,7 *",
			[]time.Time{
				timeParse("2025-02-01T19:01:00Z"),
				timeParse("2025-02-16T19:01:00Z"),
				timeParse("2025-02-28T19:01:00Z"),
				timeParse("2025-07-01T19:01:00Z"),
				timeParse("2025-07-16T19:01:00Z"),
				timeParse("2025-07-31T19:01:00Z"),
			},
		},
		{"1 19 * * 1#1,7L",
			[]time.Time{
				timeParse("2025-01-06T19:01:00Z"),
				timeParse("2025-01-26T19:01:00Z"),
				timeParse("2025-02-03T19:01:00Z"),
				timeParse("2025-02-23T19:01:00Z"),
				timeParse("2025-03-03T19:01:00Z"),
				timeParse("2025-03-30T19:01:00Z"),
				timeParse("2025-04-07T19:01:00Z"),
				timeParse("2025-04-27T19:01:00Z"),
				timeParse("2025-05-05T19:01:00Z"),
				timeParse("2025-05-25T19:01:00Z"),
				timeParse("2025-06-02T19:01:00Z"),
				timeParse("2025-06-29T19:01:00Z"),
				timeParse("2025-07-07T19:01:00Z"),
				timeParse("2025-07-27T19:01:00Z"),
				timeParse("2025-08-04T19:01:00Z"),
				timeParse("2025-08-31T19:01:00Z"),
				timeParse("2025-09-01T19:01:00Z"),
				timeParse("2025-09-28T19:01:00Z"),
				timeParse("2025-10-06T19:01:00Z"),
				timeParse("2025-10-26T19:01:00Z"),
				timeParse("2025-11-03T19:01:00Z"),
				timeParse("2025-11-30T19:01:00Z"),
				timeParse("2025-12-01T19:01:00Z"),
				timeParse("2025-12-28T19:01:00Z"),
			},
		},
		{"1 19 10-13/3 * *",
			[]time.Time{
				timeParse("2025-01-10T19:01:00Z"),
				timeParse("2025-01-13T19:01:00Z"),
				timeParse("2025-02-10T19:01:00Z"),
				timeParse("2025-02-13T19:01:00Z"),
				timeParse("2025-03-10T19:01:00Z"),
				timeParse("2025-03-13T19:01:00Z"),
				timeParse("2025-04-10T19:01:00Z"),
				timeParse("2025-04-13T19:01:00Z"),
				timeParse("2025-05-10T19:01:00Z"),
				timeParse("2025-05-13T19:01:00Z"),
				timeParse("2025-06-10T19:01:00Z"),
				timeParse("2025-06-13T19:01:00Z"),
				timeParse("2025-07-10T19:01:00Z"),
				timeParse("2025-07-13T19:01:00Z"),
				timeParse("2025-08-10T19:01:00Z"),
				timeParse("2025-08-13T19:01:00Z"),
				timeParse("2025-09-10T19:01:00Z"),
				timeParse("2025-09-13T19:01:00Z"),
				timeParse("2025-10-10T19:01:00Z"),
				timeParse("2025-10-13T19:01:00Z"),
				timeParse("2025-11-10T19:01:00Z"),
				timeParse("2025-11-13T19:01:00Z"),
				timeParse("2025-12-10T19:01:00Z"),
				timeParse("2025-12-13T19:01:00Z"),
			},
		},
	}
	for _, c := range cases {
		t.Run(c.spec, func(t *testing.T) {
			job := gen.CronJob{Name: "testJob", Spec: c.spec}
			mask, err := cronParseSpec(job)
			if err != nil {
				t.Fatal(err)
			}
			now, _ := time.Parse(time.RFC3339, "2025-01-01T00:00:00Z")
			out := []time.Time{}
			for i := 0; i < 60*24*365; i++ {
				now = now.Add(time.Minute)
				if mask.IsRunAt(now) == false {
					continue
				}
				out = append(out, now)
			}
			if reflect.DeepEqual(out, c.out) == false {
				t.Fatalf("mismatch result")
			}
		})
	}
}

func TestCronParseSpecField(t *testing.T) {
	cases := []testCaseCronSpecField{
		// min
		// allowed: *, d, d-d, */d
		{"wildcard", "*", cronFieldMin,
			[]cronMask{},
			nil,
		},
		{"seq", "0,59,3", cronFieldMin,
			[]cronMask{
				cronMaskTypeMin | 1 | 1<<59 | 1<<3,
			},
			nil,
		},
		{"seq-err", "0,a,3", cronFieldMin,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q",
				"a", "0,a,3"),
		},
		{"seq-err", "0,59,63", cronFieldMin,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q: %d > %d",
				"63", "0,59,63", 63, cronFieldMin.max),
		},
		{"range", "3-33", cronFieldMin,
			[]cronMask{
				cronMaskTypeMin | 17179869176,
			},
			nil,
		},
		{"range-err", "33-3", cronFieldMin,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q: %d(end) must be greater %d(start)",
				"33-3", "33-3", 3, 33),
		},
		{"interval", "*/7", cronFieldMin,
			[]cronMask{
				cronMaskTypeMin | 72624976668147841,
			},
			nil,
		},
		{"interval-err", "*/70", cronFieldMin,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q: %d > %d",
				"*/70", "*/70", 70, cronFieldMin.max),
		},
		{"wildcard-interval-err", "*/7,*", cronFieldMin,
			[]cronMask{},
			fmt.Errorf("wildcard '*' is used along with the others in %q",
				"*/7,*"),
		},
		{"minute-range-interval", "2-30/2", cronFieldMin,
			[]cronMask{
				cronMaskTypeMin | 1431655764,
			},
			nil,
		},
		{"minute-range-interval", "2-60/2", cronFieldMin,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q: %d > %d",
				"60", "2-60/2", 60, cronFieldMin.max),
		},
		{"hour-range-interval", "1-23/2", cronFieldHour,
			[]cronMask{
				cronMaskTypeHour | 11184810,
			},
			nil,
		},
		{"minute-err", "7,L", cronFieldMin,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q", "L", "7,L"),
		},
		{"minute-err", "7,3L", cronFieldMin,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q", "3L", "7,3L"),
		},
		{"minute-err", "7,2#2", cronFieldMin,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q", "2#2", "7,2#2"),
		},
		{"hour-err", "25", cronFieldHour,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q: %d > %d", "25", "25",
				25, cronFieldHour.max),
		},
		{"day", "L", cronFieldDay,
			[]cronMask{
				cronMaskTypeLastDM,
			},
			nil,
		},
		{"day", "L,3,1", cronFieldDay,
			[]cronMask{
				cronMaskTypeDay | 10,
				cronMaskTypeLastDM,
			},
			nil,
		},
		{"day", "3-6,L,3,1", cronFieldDay,
			[]cronMask{
				cronMaskTypeDay | 1<<1 | 1<<3 | 1<<4 | 1<<5 | 1<<6,
				cronMaskTypeLastDM,
			},
			nil,
		},
		{"day-err", "0,30", cronFieldDay,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q: %d < %d",
				"0", "0,30", 0, cronFieldDay.min),
		},
		{"day-err", "1,33", cronFieldDay,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q: %d > %d",
				"33", "1,33", 33, cronFieldDay.max),
		},
		{"weekday", "3L", cronFieldWeekDay,
			[]cronMask{
				cronMaskTypeLastDW | 3,
			},
			nil,
		},
		{"weekday", "0,7", cronFieldWeekDay, // 0 and 7 stand for Sunday
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q: %d < %d", "0", "0,7",
				0, cronFieldWeekDay.min),
		},
		{"weekday", "5#4", cronFieldWeekDay, // 0 and 7 stand for Sunday
			[]cronMask{
				cronMaskTypeNDW | 5<<8 | 4,
			},
			nil,
		},
		{"weekday", "2#3,3#2", cronFieldWeekDay, // 0 and 7 stand for Sunday
			[]cronMask{
				cronMaskTypeNDW | 2<<8 | 3,
				cronMaskTypeNDW | 3<<8 | 2,
			},
			nil,
		},
		{"weekday-err", "0#4", cronFieldWeekDay,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q",
				"0#4", "0#4"),
		},
		{"weekday-err", "*/4", cronFieldWeekDay,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q",
				"*/4", "*/4"),
		},
		{"weekday-err", "1#6", cronFieldWeekDay,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q",
				"1#6", "1#6"),
		},
		{"weekday-err", "#1", cronFieldWeekDay,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q",
				"#1", "#1"),
		},
		{"weekday-err", "1#", cronFieldWeekDay,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q",
				"1#", "1#"),
		},
		{"weekday", "3L,1,5", cronFieldWeekDay,
			[]cronMask{
				cronMaskTypeWeekDay | 1<<1 | 1<<5,
				cronMaskTypeLastDW | 3,
			},
			nil,
		},
		{"weekday", "2-6,1#5,7L", cronFieldWeekDay,
			[]cronMask{
				cronMaskTypeWeekDay | 1<<2 | 1<<3 | 1<<4 | 1<<5 | 1<<6,
				cronMaskTypeNDW | 1<<8 | 5,
				cronMaskTypeLastDW | 7,
			},
			nil,
		},
		{"weekday-err", "1,8", cronFieldWeekDay,
			[]cronMask{},
			fmt.Errorf("incorrect value: %v in %q: %d > %d",
				"8", "1,8", 8, cronFieldWeekDay.max),
		},
	}

	for _, c := range cases {
		t.Run(c.name+":"+c.in, func(t *testing.T) {
			out, err := cronParseSpecField(c.in, c.field)
			if err != nil {
				if c.outerr != nil {
					if err.Error() == c.outerr.Error() {
						return
					}
					t.Fatalf("exp: <<%v>> got: <<%v>>", c.outerr, err)
				}
				t.Fatal(err)
			}
			if reflect.DeepEqual(out, c.out) == false {
				t.Fatalf("error in %q. exp: %v got: %v", c.in, c.out, out)
			}

		})
	}
}

func TestCronOverlapFix(t *testing.T) {
	// Test case: "0 12 15 * 1" - should run at noon on 15th of any month OR on Mondays
	// This tests the fix for the overlap bug where both day and weekday are specified
	job := gen.CronJob{Name: "testOverlapJob", Spec: "0 12 15 * 1"}
	spec, err := cronParseSpec(job)
	if err != nil {
		t.Fatal(err)
	}

	// Test dates for January 2024
	// Monday, Jan 15, 2024 12:00 - should match (both day=15 AND weekday=Monday)
	jan15_2024 := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	if !spec.IsRunAt(jan15_2024) {
		t.Errorf("Expected Jan 15, 2024 12:00 (Monday, 15th) to match '0 12 15 * 1'")
	}

	// Monday, Jan 8, 2024 12:00 - should match (weekday=Monday, even though day≠15)
	jan8_2024 := time.Date(2024, 1, 8, 12, 0, 0, 0, time.UTC)
	if !spec.IsRunAt(jan8_2024) {
		t.Errorf("Expected Jan 8, 2024 12:00 (Monday, 8th) to match '0 12 15 * 1' - weekday should match")
	}

	// Tuesday, Jan 15, 2024 12:00 - should match (day=15, even though weekday≠Monday)
	jan15_2024_tue := time.Date(2024, 1, 16, 12, 0, 0, 0, time.UTC) // 16th is Tuesday, but let's use 15th
	jan15_2024_tue = time.Date(2024, 2, 15, 12, 0, 0, 0, time.UTC)  // Feb 15, 2024 is Thursday
	if !spec.IsRunAt(jan15_2024_tue) {
		t.Errorf("Expected Feb 15, 2024 12:00 (Thursday, 15th) to match '0 12 15 * 1' - day should match")
	}

	// Tuesday, Jan 9, 2024 12:00 - should NOT match (neither day=15 nor weekday=Monday)
	jan9_2024 := time.Date(2024, 1, 9, 12, 0, 0, 0, time.UTC)
	if spec.IsRunAt(jan9_2024) {
		t.Errorf("Expected Jan 9, 2024 12:00 (Tuesday, 9th) to NOT match '0 12 15 * 1'")
	}

	// Test different time - should not match due to hour constraint
	jan15_2024_wrong_hour := time.Date(2024, 1, 15, 13, 0, 0, 0, time.UTC)
	if spec.IsRunAt(jan15_2024_wrong_hour) {
		t.Errorf("Expected Jan 15, 2024 13:00 to NOT match '0 12 15 * 1' due to hour constraint")
	}
}

func TestCronCornerCases(t *testing.T) {
	// Test last weekday of month (xL) - this will expose the missing return false bug
	job := gen.CronJob{Name: "testLastWeekday", Spec: "0 12 * * 1L"} // Last Monday of month at 12:00
	spec, err := cronParseSpec(job)
	if err != nil {
		t.Fatal(err)
	}

	// January 2024: Last Monday is Jan 29, 2024
	lastMonday := time.Date(2024, 1, 29, 12, 0, 0, 0, time.UTC)
	if !spec.IsRunAt(lastMonday) {
		t.Errorf("Expected Jan 29, 2024 12:00 (last Monday) to match '0 12 * * 1L'")
	}

	// January 2024: Jan 22, 2024 is Monday but NOT the last Monday
	notLastMonday := time.Date(2024, 1, 22, 12, 0, 0, 0, time.UTC)
	if spec.IsRunAt(notLastMonday) {
		t.Errorf("Expected Jan 22, 2024 12:00 (Monday but not last) to NOT match '0 12 * * 1L'")
	}

	// Test nth weekday of month (w#n) - this will expose the missing return false bug
	job2 := gen.CronJob{Name: "testNthWeekday", Spec: "0 12 * * 1#2"} // 2nd Monday of month at 12:00
	spec2, err := cronParseSpec(job2)
	if err != nil {
		t.Fatal(err)
	}

	// January 2024: 2nd Monday is Jan 8, 2024
	secondMonday := time.Date(2024, 1, 8, 12, 0, 0, 0, time.UTC)
	if !spec2.IsRunAt(secondMonday) {
		t.Errorf("Expected Jan 8, 2024 12:00 (2nd Monday) to match '0 12 * * 1#2'")
	}

	// January 2024: Jan 15, 2024 is 3rd Monday, not 2nd
	thirdMonday := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	if spec2.IsRunAt(thirdMonday) {
		t.Errorf("Expected Jan 15, 2024 12:00 (3rd Monday, not 2nd) to NOT match '0 12 * * 1#2'")
	}

	// January 2024: Jan 1, 2024 is 1st Monday, not 2nd
	firstMonday := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	if spec2.IsRunAt(firstMonday) {
		t.Errorf("Expected Jan 1, 2024 12:00 (1st Monday, not 2nd) to NOT match '0 12 * * 1#2'")
	}

	// Test leap year handling for last day of month (L)
	jobL := gen.CronJob{Name: "testLastDay", Spec: "0 12 L * *"} // Last day of month at 12:00
	specL, err := cronParseSpec(jobL)
	if err != nil {
		t.Fatal(err)
	}

	// 2024 is a leap year - February has 29 days
	feb29_2024 := time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)
	if !specL.IsRunAt(feb29_2024) {
		t.Errorf("Expected Feb 29, 2024 12:00 (last day of leap year February) to match '0 12 L * *'")
	}

	// 2024 leap year - February 28th should NOT match (not the last day)
	feb28_2024 := time.Date(2024, 2, 28, 12, 0, 0, 0, time.UTC)
	if specL.IsRunAt(feb28_2024) {
		t.Errorf("Expected Feb 28, 2024 12:00 (not last day in leap year) to NOT match '0 12 L * *'")
	}

	// 2023 is NOT a leap year - February has 28 days
	feb28_2023 := time.Date(2023, 2, 28, 12, 0, 0, 0, time.UTC)
	if !specL.IsRunAt(feb28_2023) {
		t.Errorf("Expected Feb 28, 2023 12:00 (last day of non-leap year February) to match '0 12 L * *'")
	}

	// 2023 non-leap year - February 27th should NOT match (not the last day)
	feb27_2023 := time.Date(2023, 2, 27, 12, 0, 0, 0, time.UTC)
	if specL.IsRunAt(feb27_2023) {
		t.Errorf("Expected Feb 27, 2023 12:00 (not last day in non-leap year) to NOT match '0 12 L * *'")
	}
}

// func TestCronSchedule(t *testing.T) {
//
// 	c := createCron(&mockCronNode{})
// 	defer c.terminate()
//
// 	j1 := gen.CronJob{
// 		Name: "testCron1",
// 		Spec: "* * * * *",
// 	}
//
// 	if err := c.AddJob(j1); err != nil {
// 		t.Fatal(err)
// 	}
// }
//
// type mockLog struct{}
//
// func (l *mockLog) Level() gen.LogLevel                { return gen.LogLevelInfo }
// func (l *mockLog) SetLevel(level gen.LogLevel) error  { return nil }
// func (l *mockLog) Logger() string                     { return "" }
// func (l *mockLog) SetLogger(name string)              {}
// func (l *mockLog) Trace(format string, args ...any)   {}
// func (l *mockLog) Debug(format string, args ...any)   {}
// func (l *mockLog) Info(format string, args ...any)    {}
// func (l *mockLog) Warning(format string, args ...any) {}
// func (l *mockLog) Error(format string, args ...any)   { panic(fmt.Sprintf(format, args...)) }
// func (l *mockLog) Panic(format string, args ...any)   { panic(fmt.Sprintf(format, args...)) }
//
// type mockCronNode struct {
// 	name gen.Atom
// }
//
// func (mcn *mockCronNode) Name() gen.Atom {
// 	return mcn.name
// }
//
// func (mcn *mockCronNode) IsAlive() bool { return true }
//
// func (mcn *mockCronNode) Log() gen.Log {
// 	return &mockLog{}
// }
//
// func (mcn *mockCronNode) Send(to any, message any) error {
// 	return nil
// }
// func (mcn *mockCronNode) Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
// 	var pid gen.PID
// 	return pid, nil
// }
// func (mcn *mockCronNode) SpawnRegister(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
// 	var pid gen.PID
// 	return pid, nil
// }
