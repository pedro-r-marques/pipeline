package pipeline

import (
	"container/heap"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Cron defines the interface to periodically trigger tasks
type Cron interface {
	Add(name string, sched *CronSchedule, callback func()) error
	List() map[string]*CronSchedule
	Delete(name string) error
}

// CronSchedule defines a crontab(5) entry
type CronSchedule struct {
	Min     string
	Hour    string
	Day     string
	Month   string
	Weekday string
}

type schedule struct {
	min      []int
	hour     []int
	dayMonth []int
	month    []int
	weekday  []int
}

type cronEntry struct {
	expireTime time.Time
	sched      *CronSchedule
	repr       *schedule
	callback   func()
}

type cronHeap []*cronEntry

func (h cronHeap) Len() int           { return len(h) }
func (h cronHeap) Less(i, j int) bool { return h[i].expireTime.Before(h[j].expireTime) }
func (h cronHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *cronHeap) Push(x interface{}) {
	*h = append(*h, x.(*cronEntry))
}

func (h *cronHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func getValueInSet(x int, set []int) int {
	if set == nil {
		return x
	}
	for _, v := range set {
		if v >= x {
			return v
		}
	}
	return set[0]
}

var (
	monthDays = []int{
		31, // jan
		28, // feb
		31, // mar
		30, // apr
		31, // may
		30, // jun
		31, // jul
		31, // aug
		30, // sep
		31, // oct
		30, // nov
		31, // dec
	}
)

type cDate struct {
	year  int
	month int
	day   int
	hour  int
	min   int
}

func (d *cDate) incMonth() bool {
	d.month++
	if d.month > 12 {
		d.month = 1
		d.year++
		return true
	}
	return false
}

func (d *cDate) incDay(delta int) bool {
	d.day += delta
	if d.day > monthDays[d.month-1] {
		d.day = 1
		d.incMonth()
		return true
	}
	return false
}

func (d *cDate) incHour() bool {
	d.hour++
	if d.hour >= 24 {
		d.hour = 0
		d.incDay(1)
		return true
	}
	return false
}

func (d *cDate) incMin() bool {
	d.min++
	if d.min >= 60 {
		d.min = 0
		d.incHour()
		return true
	}
	return false
}

func (d *cDate) weekday() int {
	dt := time.Date(d.year, time.Month(d.month), d.day, 0, 0, 0, 0, time.UTC)
	return int(dt.Weekday())
}

func nextExpiryTime(current time.Time, r *schedule) time.Time {
	var dt cDate
	var m time.Month
	dt.year, m, dt.day = current.Date()
	dt.month = int(m)
	dt.hour = current.Hour()
	dt.min = current.Minute()

	for i := 0; ; i++ {
		next := dt
		if m := getValueInSet(next.month, r.month); m != next.month {
			next.month = m
			next.day, next.hour, next.min = 1, 0, 0
			if next.month < dt.month {
				next.year++
			}
		}

		if d := getValueInSet(next.day, r.dayMonth); d != next.day {
			carry := d < next.day
			next.day = d
			next.hour, next.min = 0, 0
			if carry {
				next.incMonth()
				dt = next
				continue
			}
		} else if r.weekday != nil {
			wkD := next.weekday()
			if wkN := getValueInSet(wkD, r.weekday); wkN != wkD {
				next.hour, next.min = 0, 0
				var delta int
				if wkN < wkD {
					delta = (7 - wkD) + wkN
				} else {
					delta = wkN - wkD
				}
				if next.incDay(delta) {
					dt = next
					continue
				}
			}
		}

		if h := getValueInSet(next.hour, r.hour); h != next.hour {
			carry := h < next.hour
			next.hour = h
			next.min = 0
			if carry {
				next.incDay(1)
				dt = next
				continue
			}
		}

		if i == 0 && next == dt && next.incMin() {
			dt = next
			continue
		}

		if m := getValueInSet(next.min, r.min); m != next.min {
			carry := m < next.min
			next.min = m
			if carry {
				next.incHour()
				dt = next
				continue
			}
		}

		return time.Date(next.year, time.Month(next.month), next.day, next.hour, next.min, 0, 0, time.UTC)
	}
}

type cronExecutor struct {
	sync.Mutex
	timer      *time.Timer
	entries    map[string]*cronEntry
	expireHeap cronHeap
	now        func() time.Time
}

func parseEntry(s string, keyValues map[string]int) ([]int, error) {
	if s == "" || s == "*" {
		return nil, nil
	}
	var values []int
	for _, element := range strings.Split(s, ",") {
		// look for keywords
		if keyValues != nil {
			if v, ok := keyValues[element]; ok {
				values = append(values, v)
				continue
			}
		}
		// ranges
		if parts := strings.Split(element, "-"); len(parts) == 2 {
			v1, err := strconv.Atoi(parts[0])
			if err != nil {
				return nil, err
			}
			v2, err := strconv.Atoi(parts[1])
			if err != nil {
				return nil, err
			}
			for i := v1; i <= v2; i++ {
				values = append(values, i)
			}
			continue
		}
		v, err := strconv.Atoi(element)
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	if values != nil {
		sort.Ints(values)
	}
	for i, v := range values {
		if i == 0 {
			continue
		}
		if v == values[i-1] {
			return nil, fmt.Errorf("duplicate value %d", v)
		}
	}
	return values, nil
}

var (
	daysOfWeek = map[string]int{
		"sun": int(time.Sunday),
		"mon": int(time.Monday),
		"tue": int(time.Tuesday),
		"wed": int(time.Wednesday),
		"thu": int(time.Thursday),
		"fri": int(time.Friday),
		"sat": int(time.Saturday),
	}
	months = map[string]int{
		"jan": 1,
		"feb": 2,
		"mar": 3,
		"apr": 4,
		"may": 5,
		"jun": 6,
		"jul": 7,
		"aug": 8,
		"sep": 9,
		"oct": 10,
		"nov": 11,
		"dec": 12,
	}
)

func parseCron(s *CronSchedule) (*schedule, error) {
	repr := new(schedule)
	var err error
	if repr.min, err = parseEntry(s.Min, nil); err != nil {
		return nil, err
	}
	if repr.hour, err = parseEntry(s.Hour, nil); err != nil {
		return nil, err
	}
	if repr.dayMonth, err = parseEntry(s.Day, nil); err != nil {
		return nil, err
	}
	if repr.month, err = parseEntry(s.Month, months); err != nil {
		return nil, err
	}
	if repr.weekday, err = parseEntry(s.Weekday, daysOfWeek); err != nil {
		return nil, err
	}

	return repr, nil
}

func (c *cronExecutor) timerHandler() {
	c.Lock()
	i := heap.Pop(&c.expireHeap)
	c.Unlock()

	entry := i.(*cronEntry)
	if entry.callback != nil {
		entry.callback()
		entry.expireTime = nextExpiryTime(entry.expireTime, entry.repr)
		// glog.V(3).Infof("next expiration %s", entry.expireTime.String())
	}

	c.Lock()
	defer c.Unlock()
	if entry.callback != nil {
		heap.Push(&c.expireHeap, entry)
	}
	next := c.expireHeap[0]
	c.timer = time.AfterFunc(next.expireTime.Sub(c.now()), c.timerHandler)
}

func (c *cronExecutor) Add(name string, sched *CronSchedule, callback func()) error {
	r, err := parseCron(sched)
	if err != nil {
		return err
	}

	entry := &cronEntry{sched: sched, repr: r, callback: callback}
	entry.expireTime = nextExpiryTime(c.now(), entry.repr)
	// glog.V(3).Infof("%s expires at %s", name, entry.expireTime.String())

	c.Lock()
	defer c.Unlock()

	c.entries[name] = entry
	heap.Push(&c.expireHeap, entry)
	if c.expireHeap[0] == entry {
		if c.timer != nil {
			c.timer.Stop()
		}
		c.timer = time.AfterFunc(entry.expireTime.Sub(c.now()), c.timerHandler)
	}
	return nil
}

func (c *cronExecutor) List() map[string]*CronSchedule {
	c.Lock()
	defer c.Unlock()

	response := make(map[string]*CronSchedule)
	for k, v := range c.entries {
		response[k] = v.sched
	}
	return response
}
func (c *cronExecutor) Delete(name string) error {
	c.Lock()
	defer c.Unlock()
	entry, ok := c.entries[name]
	if !ok {
		return fmt.Errorf("cron %s not found", name)
	}
	delete(c.entries, name)
	entry.callback = nil
	for i := range c.expireHeap {
		if c.expireHeap[i] == entry {
			heap.Remove(&c.expireHeap, i)
			if i == 0 {
				c.timer.Stop()
				if len(c.expireHeap) > 0 {
					next := c.expireHeap[0]
					c.timer = time.AfterFunc(next.expireTime.Sub(c.now()), c.timerHandler)
				}
			}
			break
		}
	}

	return nil
}

// NewCronExecutor allocates an object that implements the Cron interface
func NewCronExecutor() Cron {
	exec := &cronExecutor{
		entries: make(map[string]*cronEntry),
		now:     func() time.Time { return time.Now().UTC() },
	}
	heap.Init(&exec.expireHeap)
	return exec
}
