package node

import (
	"ergo.services/ergo/gen"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type cronField struct {
	min  int
	max  int
	reg  *regexp.Regexp
	mask cronMask // default mask
}

type cronMask uint64
type cronMaskList []cronMask
type cronSpecMask struct {
	MinHourMonth cronMaskList
	Day          cronMaskList
	WeekDay      cronMaskList
}

const (
	// use 4 bits (0..15)
	// cronMaskTypeInvalid = 0
	cronMaskTypeLastDM = 1 << 60 // 'L' in day of month
	cronMaskTypeLastDW = 2 << 60 // 'xL' in day of week
	cronMaskTypeNDW    = 3 << 60 // 'day_of_week#occurrence' in day of week

	cronMaskTypeMin     = 10 << 60
	cronMaskTypeHour    = 11 << 60
	cronMaskTypeDay     = 12 << 60
	cronMaskTypeMonth   = 13 << 60
	cronMaskTypeWeekDay = 14 << 60
	cronMaskType        = 15 << 60
)

func (csm cronSpecMask) IsRunAt(t time.Time) bool {
	if len(csm.Day) == 0 && csm.WeekDay.IsRunAt(t) == false {
		return false
	}

	if len(csm.WeekDay) == 0 && csm.Day.IsRunAt(t) == false {
		return false
	}

	if csm.Day.IsRunAt(t) == false && csm.WeekDay.IsRunAt(t) == false {
		return false
	}

	if csm.MinHourMonth.IsRunAt(t) == false {
		return false
	}

	return true
}

func (cml cronMaskList) IsRunAt(t time.Time) bool {
	run := true
	for _, cm := range cml {
		switch cm.MaskType() {
		case cronMaskTypeMin, cronMaskTypeHour, cronMaskTypeMonth:
			if cm.IsRunAt(t) == false {
				return false
			}
			continue
		}
		run = cm.IsRunAt(t)
		if run == true {
			return true
		}

	}
	return run
}

func (cm cronMask) MaskType() uint64 {
	return uint64(cm & cronMaskType)
}

func (cm cronMask) IsRunAt(t time.Time) bool {
	switch cm.MaskType() {
	case cronMaskTypeMin:
		m := t.Minute()
		if cm&(1<<m) > 0 {
			return true
		}

	case cronMaskTypeHour:
		h := t.Hour()
		if cm&(1<<h) > 0 {
			return true
		}

	case cronMaskTypeDay:
		d := t.Day()
		if cm&(1<<d) > 0 {
			return true
		}

	case cronMaskTypeMonth:
		m := t.Month()
		if cm&(1<<m) > 0 {
			return true
		}

	case cronMaskTypeWeekDay:
		wd := t.Weekday()
		if wd == 0 {
			wd = 7
		}
		if cm&(1<<wd) > 0 {
			return true
		}

	case cronMaskTypeLastDM:
		last := t.AddDate(0, 1, -t.Day()).Day()
		return last == t.Day()

	case cronMaskTypeLastDW:
		wd := int(cm & 15)
		twd := int(t.Weekday())
		if twd == 0 {
			twd = 7
		}
		if wd != twd {
			return false
		}
		tm := t.Month()
		m := t.Add(time.Hour * 7 * 24).Month()
		if tm != m {
			return true
		}

	case cronMaskTypeNDW:
		twd := int(t.Weekday())
		if twd == 0 {
			twd = 7
		}
		wd := int((cm >> 8) & 255)
		if wd != twd {
			return false
		}
		n := int(cm & 255)
		tn := ((t.Day() - 1) / 7) + 1
		if tn == n {
			return true
		}

	default:
		// shouldnt happen
		e := fmt.Sprintf("unknown cronMaskType %d", cm.MaskType()>>60)
		panic(e)
	}
	return false
}

var (
	cronFieldMin = cronField{
		min:  0,
		max:  59,
		mask: cronMaskTypeMin,
		// allowed: *, d, d-d, */d, d-d/d
		reg: regexp.MustCompile(`^(?:\*$|\*/\d+|\d+-\d+|\d+-\d+/\d+|\d+)$`),
	}
	cronFieldHour = cronField{
		min:  0,
		max:  23,
		mask: cronMaskTypeHour,
		// allowed: *, d, d-d, */d, d-d/d
		reg: regexp.MustCompile(`^(?:\*$|\*/\d+|\d+-\d+|\d+-\d+/\d+|\d+)$`),
	}
	cronFieldDay = cronField{
		min:  1,
		max:  31,
		mask: cronMaskTypeDay,
		// allowed: *, d, d-d, */d, d-d/d, L
		reg: regexp.MustCompile(`^(?:\*$|\*/\d+|\d+-\d+|\d+-\d+/\d+|L|\d+)$`),
	}
	cronFieldMonth = cronField{
		min:  1,
		max:  12,
		mask: cronMaskTypeMonth,
		// allowed: *, d, d-d, */d
		reg: regexp.MustCompile(`^(?:\*$|\*/\d+|\d+-\d+|\d+)$`),
	}
	cronFieldWeekDay = cronField{
		min:  1,
		max:  7,
		mask: cronMaskTypeWeekDay,
		// allowed: * or d, d-d, dL, d#d
		reg: regexp.MustCompile(`^(?:\*$|\d+-\d+|[1-7]L|\d+|[1-7]#[1-5])$`),
	}
)

func cronParseSpec(job gen.CronJob) (cronSpecMask, error) {
	var mask cronSpecMask
	spec := job.Spec

	// @daily, @hourly, @monthly, @weekly
	switch job.Spec {
	case "@hourly":
		spec = "1 * * * *"
	case "@daily":
		spec = "10 3 * * *" // everyday at 3:10
	case "@monthly":
		spec = "20 4 1 * *" // on day 1 of the month at 4:20
	case "@weekly":
		spec = "30 5 * * 1" // on monday at 5:30
	}

	// parse a standart cron format: min hour day month weekday
	//  * * * * *
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return mask, fmt.Errorf("incorrect cron spec format")
	}

	// parse min field
	m, err := cronParseSpecField(fields[0], cronFieldMin)
	if err != nil {
		return mask, err
	}
	mask.MinHourMonth = append(mask.MinHourMonth, m...)

	// parse hour field
	m, err = cronParseSpecField(fields[1], cronFieldHour)
	if err != nil {
		return mask, err
	}
	mask.MinHourMonth = append(mask.MinHourMonth, m...)

	// parse month field
	m, err = cronParseSpecField(fields[3], cronFieldMonth)
	if err != nil {
		return mask, err
	}
	mask.MinHourMonth = append(mask.MinHourMonth, m...)

	// parse day field
	m, err = cronParseSpecField(fields[2], cronFieldDay)
	if err != nil {
		return mask, err
	}
	mask.Day = m

	// parse weekday field
	m, err = cronParseSpecField(fields[4], cronFieldWeekDay)
	if err != nil {
		return mask, err
	}
	mask.WeekDay = m

	return mask, nil
}

func cronParseSpecField(f string, field cronField) (cronMaskList, error) {
	// default mask with cleaned bits
	result := cronMaskList{field.mask}
	mtype := field.mask

	fieldOptions := strings.Split(f, ",")
	for _, fo := range fieldOptions {
		submatch := field.reg.FindStringSubmatch(fo)
		if len(submatch) == 0 {
			return nil, fmt.Errorf("incorrect value: %v in %q", fo, f)
		}
		for _, sm := range submatch {

			switch sm {
			case "*":
				if len(fieldOptions) > 1 {
					return nil, fmt.Errorf("wildcard '*' is used along with the others in %q", f)
				}
				// return empty mask
				return cronMaskList{}, nil

			case "L":
				// last day of month/week
				if mtype == cronMaskTypeDay {
					result = append(result, cronMaskTypeLastDM)
					continue
				}

				// shouldnt reach this code - protected by regexp.
				// if we are here - something is wrong with regexp
				err := fmt.Errorf("incorrect regexp for the cron field type %d in %q", mtype, f)
				panic(err)
			}

			// interval */x
			if l := strings.Split(sm, "*/"); len(l) == 2 {
				step, err := cronParseInt(l[1], 1, field.max)
				if err != nil {
					return nil, fmt.Errorf("incorrect value: %v in %q: %w", fo, f, err)
				}
				for x := field.min; x <= field.max; x += step {
					result[0] |= 1 << x
				}
				continue
			}

			// range
			if l := strings.Split(sm, "-"); len(l) == 2 {
				step := 1
				rangeStart, err := cronParseInt(l[0], field.min, field.max)
				if err != nil {
					return nil, fmt.Errorf("incorrect value: %v in %q: %w", fo, f, err)
				}
				rangeEnd := 0
				if r := strings.Split(l[1], "/"); len(r) == 2 {
					// interval within a range [d-d/d]
					// example 1-31/2 - every odd days
					//         2-30/2 - every even days
					rend, err := cronParseInt(r[0], field.min, field.max)
					if err != nil {
						return nil, fmt.Errorf("incorrect value: %v in %q: %w", r[0], f, err)
					}
					rangeEnd = rend
					st, err := cronParseInt(r[1], 1, field.max)
					if err != nil {
						return nil, fmt.Errorf("incorrect value: %v in %q: %w", r[1], f, err)
					}
					step = st
				} else {
					rend, err := cronParseInt(l[1], field.min, field.max)
					if err != nil {
						return nil, fmt.Errorf("incorrect value: %v in %q: %w", fo, f, err)
					}
					rangeEnd = rend
				}
				if rangeStart > rangeEnd {
					return nil, fmt.Errorf("incorrect value: %v in %q: %d(end) must be greater %d(start)",
						fo, f, rangeEnd, rangeStart)
				}
				for x := rangeStart; x <= rangeEnd; x += step {
					result[0] |= 1 << x
				}
				continue
			}

			// w#n
			if l := strings.Split(sm, "#"); len(l) == 2 {
				w, err := cronParseInt(l[0], field.min, field.max)
				if err != nil {
					return nil, fmt.Errorf("incorrect value: %v in %q: %w", fo, f, err)
				}
				n, err := cronParseInt(l[1], field.min, field.max)
				if err != nil {
					return nil, fmt.Errorf("incorrect value: %v in %q: %w", fo, f, err)
				}
				ndw := cronMaskTypeNDW
				ndw |= w << 8
				ndw |= n
				result = append(result, cronMask(ndw))
				continue
			}

			// xL last x (day of week) of a given month
			if l := strings.Split(sm, "L"); len(l) == 2 {
				n, err := cronParseInt(l[0], 1, 7)
				if err != nil {
					err = fmt.Errorf("incorrect regexp for the cron field type %d in %q", mtype, f)
					panic(err)
				}
				switch mtype {
				case cronMaskTypeDay:
					xl := cronMaskTypeLastDM | n
					result = append(result, cronMask(xl))
					continue
				case cronMaskTypeWeekDay:
					xl := cronMaskTypeLastDW | n
					result = append(result, cronMask(xl))
					continue
				}
				err = fmt.Errorf("incorrect regexp for the cron field type %d in %q", mtype, f)
				panic(err)
			}

			// number
			n, err := cronParseInt(sm, field.min, field.max)
			if err != nil {
				return nil, fmt.Errorf("incorrect value: %v in %q: %w", fo, f, err)
			}

			result[0] |= 1 << n
		}
	}

	// check if the first element has an empty mask
	// (for instance, day field has 'L' only)
	if result[0] == field.mask&cronMaskType {
		// remove empty mask
		result = result[1:]
		if len(result) == 0 {
			// shouldnt happen, but just in case
			panic("cron internal error: there was empty mask only")
		}
	}

	return result, nil
}

func cronParseInt(s string, min int, max int) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if n < min {
		return 0, fmt.Errorf("%d < %d", n, min)
	}
	if n > max {
		return 0, fmt.Errorf("%d > %d", n, max)
	}
	return n, nil
}
