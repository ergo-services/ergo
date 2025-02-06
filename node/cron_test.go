package node

import (
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo/gen"
)

type testCaseCronSpecField struct {
	name   string
	in     string
	field  cronField
	out    []cronMask
	outerr error
}

func TestCronParseSpecField(t *testing.T) {
	cases := []testCaseCronSpecField{
		// min
		// allowed: *, d, d-d, */d
		{"wildcard", "*", cronFieldMin,
			[]cronMask{
				cronMaskTypeMin | cronMask60,
			},
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

func TestCronSchedule(t *testing.T) {

	c := createCron(&mockCronNode{})
	defer c.terminate()

	j1 := gen.CronJob{
		Name: "testCron1",
		Spec: "* * * * *",
	}

	if err := c.AddJob(j1); err != nil {
		t.Fatal(err)
	}
}

type mockLog struct{}

func (l *mockLog) Level() gen.LogLevel                { return gen.LogLevelInfo }
func (l *mockLog) SetLevel(level gen.LogLevel) error  { return nil }
func (l *mockLog) Logger() string                     { return "" }
func (l *mockLog) SetLogger(name string)              {}
func (l *mockLog) Trace(format string, args ...any)   {}
func (l *mockLog) Debug(format string, args ...any)   {}
func (l *mockLog) Info(format string, args ...any)    {}
func (l *mockLog) Warning(format string, args ...any) {}
func (l *mockLog) Error(format string, args ...any)   { panic(fmt.Sprintf(format, args...)) }
func (l *mockLog) Panic(format string, args ...any)   { panic(fmt.Sprintf(format, args...)) }

type mockCronNode struct {
	name gen.Atom
}

func (mcn *mockCronNode) Name() gen.Atom {
	return mcn.name
}

func (mcn *mockCronNode) IsAlive() bool { return true }

func (mcn *mockCronNode) Log() gen.Log {
	return &mockLog{}
}

func (mcn *mockCronNode) Send(to any, message any) error {
	return nil
}
func (mcn *mockCronNode) Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	var pid gen.PID
	return pid, nil
}
func (mcn *mockCronNode) SpawnRegister(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	var pid gen.PID
	return pid, nil
}
