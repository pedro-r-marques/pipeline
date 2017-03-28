package pipeline

import (
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestExpiryTime(t *testing.T) {
	testCases := []struct {
		when     time.Time
		expected time.Time
		spec     *CronSchedule
	}{
		{
			time.Date(2016, time.August, 11, 15, 0, 0, 0, time.UTC),
			time.Date(2016, time.August, 12, 0, 0, 0, 0, time.UTC),
			&CronSchedule{Min: "0", Hour: "0"},
		},
		{
			time.Date(2016, time.August, 11, 0, 0, 0, 0, time.UTC),
			time.Date(2016, time.August, 11, 1, 0, 0, 0, time.UTC),
			&CronSchedule{Min: "0", Hour: "0-1"},
		},
		{
			time.Date(2016, time.August, 11, 12, 0, 0, 0, time.UTC),
			time.Date(2016, time.August, 15, 0, 0, 0, 0, time.UTC),
			&CronSchedule{Min: "0", Hour: "0", Weekday: "mon"},
		},
		{
			time.Date(2016, time.August, 11, 23, 0, 0, 0, time.UTC),
			time.Date(2016, time.August, 12, 0, 0, 0, 0, time.UTC),
			&CronSchedule{Min: "0", Hour: "0"},
		},
		{
			time.Date(2016, time.August, 11, 12, 0, 0, 0, time.UTC),
			time.Date(2016, time.August, 12, 0, 0, 0, 0, time.UTC),
			&CronSchedule{Min: "0", Hour: "0", Weekday: "mon,fri"},
		},
		{
			time.Date(2016, time.August, 31, 12, 0, 0, 0, time.UTC),
			time.Date(2016, time.September, 1, 0, 0, 0, 0, time.UTC),
			&CronSchedule{Min: "0", Hour: "0"},
		},
		{
			time.Date(2016, time.August, 31, 12, 0, 0, 0, time.UTC),
			time.Date(2016, time.October, 1, 0, 0, 0, 0, time.UTC),
			&CronSchedule{Min: "0", Hour: "0", Month: "8,10"},
		},
		{
			time.Date(2016, time.October, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2016, time.October, 2, 0, 0, 0, 0, time.UTC),
			&CronSchedule{Min: "0", Hour: "0", Month: "8,10"},
		},
		{
			time.Date(2016, time.October, 31, 0, 0, 0, 0, time.UTC),
			time.Date(2017, time.August, 1, 0, 0, 0, 0, time.UTC),
			&CronSchedule{Min: "0", Hour: "0", Month: "8,10"},
		},
		{
			time.Date(2016, time.August, 31, 12, 0, 0, 0, time.UTC),
			time.Date(2016, time.September, 2, 0, 0, 0, 0, time.UTC),
			&CronSchedule{Min: "0", Hour: "0", Day: "2,15,20"},
		},
		{
			time.Date(2016, time.August, 10, 12, 0, 0, 0, time.UTC),
			time.Date(2016, time.September, 5, 0, 0, 0, 0, time.UTC),
			&CronSchedule{Min: "0", Hour: "0", Day: "5"},
		},
		{
			time.Date(2016, time.August, 11, 12, 0, 0, 0, time.UTC),
			time.Date(2016, time.August, 18, 0, 30, 0, 0, time.UTC),
			&CronSchedule{Min: "30", Hour: "0", Weekday: "thu"},
		},
		{
			time.Date(2016, time.August, 11, 12, 0, 0, 0, time.UTC),
			time.Date(2016, time.September, 1, 0, 0, 0, 0, time.UTC),
			&CronSchedule{Min: "0", Hour: "0", Month: "sep"},
		},
		{
			time.Date(2016, time.August, 11, 11, 50, 0, 0, time.UTC),
			time.Date(2016, time.August, 11, 12, 5, 0, 0, time.UTC),
			&CronSchedule{Min: "5,15,30", Hour: "10-12"},
		},
		{
			time.Date(2016, time.August, 12, 10, 30, 0, 0, time.UTC),
			time.Date(2016, time.August, 15, 0, 15, 0, 0, time.UTC),
			&CronSchedule{Min: "15", Hour: "0", Weekday: "mon,wed"},
		},
		{
			time.Date(2016, time.December, 30, 10, 30, 0, 0, time.UTC),
			time.Date(2017, time.January, 15, 0, 15, 0, 0, time.UTC),
			&CronSchedule{Min: "15", Hour: "0", Day: "15"},
		},
		{
			time.Date(2016, time.August, 12, 10, 59, 0, 0, time.UTC),
			time.Date(2016, time.August, 12, 11, 0, 0, 0, time.UTC),
			&CronSchedule{Min: "0", Day: "12"},
		},
		{
			time.Date(2016, time.August, 12, 10, 59, 0, 0, time.UTC),
			time.Date(2016, time.August, 12, 11, 0, 0, 0, time.UTC),
			&CronSchedule{Day: "12"},
		},
		{
			time.Date(2016, time.August, 12, 23, 59, 0, 0, time.UTC),
			time.Date(2016, time.August, 13, 0, 0, 0, 0, time.UTC),
			&CronSchedule{Min: "0"},
		},
		{
			time.Date(2016, time.August, 30, 10, 0, 0, 0, time.UTC),
			time.Date(2016, time.September, 5, 0, 45, 0, 0, time.UTC),
			&CronSchedule{Min: "45", Hour: "0", Weekday: "mon"},
		},
	}
	for _, test := range testCases {
		s, err := parseCron(test.spec)
		if err != nil {
			t.Error(err)
			continue
		}
		actual := nextExpiryTime(test.when, s)
		if !actual.Equal(test.expected) {
			t.Errorf("got %v, expected %v", actual, test.expected)
		}
	}
}

type cbClosure struct {
	c []int
	i int
}

func (cb *cbClosure) Inc() {
	cb.c[cb.i]++
}
func makeCallback(c []int, i int) func() {
	cl := &cbClosure{c, i}
	return cl.Inc
}

func TestCronTrigger(t *testing.T) {
	exec := NewCronExecutor()
	impl := exec.(*cronExecutor)

	timeValues := []time.Time{
		time.Date(2016, time.August, 12, 10, 10, 0, 0, time.UTC),
		time.Date(2016, time.August, 12, 10, 10, 0, 0, time.UTC),
		time.Date(2016, time.August, 12, 10, 11, 0, 0, time.UTC),
		time.Date(2016, time.August, 12, 10, 11, 0, 0, time.UTC),

		time.Date(2016, time.August, 15, 0, 15, 0, 0, time.UTC),
		time.Date(2016, time.August, 16, 0, 30, 0, 0, time.UTC),
		time.Date(2016, time.August, 17, 0, 15, 0, 0, time.UTC),
	}

	var timeNowN int
	impl.now = func() time.Time {
		var n time.Time
		if timeNowN < len(timeValues) {
			n = timeValues[timeNowN]
		}
		timeNowN++
		return n
	}

	testConfig := []*CronSchedule{
		&CronSchedule{Min: "30", Hour: "0", Day: "16"},
		&CronSchedule{Min: "15", Hour: "0", Weekday: "mon,wed"},
	}
	cbCounters := make([]int, len(testConfig))

	for i, c := range testConfig {
		fn := makeCallback(cbCounters, i)
		exec.Add("test"+strconv.Itoa(i+1), c, fn)
	}

	if timeNowN != 4 {
		t.Error(timeNowN)
	}

	for i := 0; i < 3; i++ {
		if impl.expireHeap[0].expireTime != timeValues[4+i] {
			t.Errorf("expected expiry time of %s, got %s",
				timeValues[4+i].String(),
				impl.expireHeap[0].expireTime.String())
		}

		// invoke timer.
		impl.timerHandler()
	}

	expected := []int{1, 2}
	if !reflect.DeepEqual(cbCounters, expected) {
		t.Errorf("callback counts %v", cbCounters)
	}

}

func TestCronDelete(t *testing.T) {
	exec := NewCronExecutor()
	impl := exec.(*cronExecutor)

	timeValues := []time.Time{
		time.Date(2016, time.August, 12, 10, 10, 0, 0, time.UTC),
		time.Date(2016, time.August, 12, 10, 10, 0, 0, time.UTC),
		time.Date(2016, time.August, 12, 10, 11, 0, 0, time.UTC),
		time.Date(2016, time.August, 12, 10, 11, 0, 0, time.UTC),

		time.Date(2016, time.August, 14, 10, 30, 0, 0, time.UTC),
		time.Date(2016, time.August, 16, 0, 30, 1, 0, time.UTC),
	}

	var timeNowN int
	impl.now = func() time.Time {
		var n time.Time
		if timeNowN < len(timeValues) {
			n = timeValues[timeNowN]
		}
		timeNowN++
		return n
	}

	testConfig := []*CronSchedule{
		&CronSchedule{Min: "30", Hour: "0", Day: "16"},
		&CronSchedule{Min: "15", Hour: "0", Weekday: "mon,wed"},
	}
	cbCounters := make([]int, len(testConfig))

	for i, c := range testConfig {
		fn := makeCallback(cbCounters, i)
		exec.Add("test"+strconv.Itoa(i+1), c, fn)
	}

	if len(exec.List()) != 2 {
		t.Error(exec.List())
	}

	if err := exec.Delete("test2"); err != nil {
		t.Error(err)
	}

	impl.timerHandler()

	if err := exec.Delete("test1"); err != nil {
		t.Error(err)
	}

	expected := []int{1, 0}
	if !reflect.DeepEqual(cbCounters, expected) {
		t.Errorf("callback counts %v", cbCounters)
	}

}
