///////////////////////////////////////////////////////////////////////////////////////
//gohid.go

package runtime

import "unsafe"

var goroutineHidmark int

func goroutineHid(gp *g) uintptr {
	return uintptr(unsafe.Pointer(gp)) ^ uintptr(unsafe.Pointer(&goroutineHidmark))
}
func GoroutineHid() uintptr {
	return goroutineHid(getg())
}
func goroutineHidtoData(hid uintptr) *g {
	return (*g)(unsafe.Pointer(hid ^ uintptr(unsafe.Pointer(&goroutineHidmark))))
}

func GoReady(hid uintptr) {
	goready(goroutineHidtoData(hid), 4)
}
func _unlockf(gp *g, l unsafe.Pointer) bool {
	d := (*gopark_ex)(l)
	return d.f(goroutineHid(gp), d.lock)
}

type gopark_ex struct {
	lock interface{}
	f    func(hid uintptr, lock interface{}) bool
}

func GoPark(unlockf func(hid uintptr, lock interface{}) bool, lock interface{}) {
	gopark(_unlockf, unsafe.Pointer(&gopark_ex{lock: lock, f: unlockf}), "semacquire", traceEvGoBlockSync, 4)
}

//go:linkname sleeptimer time.sleeptimer
func sleeptimer(pt **timer, ns int64, locked *int32) (bAlert bool) {
	t := *pt
	if t != nil {
		stopTimer(t)
	}
	t = &timer{f: goroutineReady, arg: getg()}
	*pt = t
	if ns < 0 {
		t.when = 0x7FFFFFFFFFFFFFFF
	} else {
		t.when = nanotime() + ns
	}
	lock(&timers.lock)
	if *locked&0x100 == 0 {
		addtimerLocked(t)
		*locked = 0
		goparkunlock(&timers.lock, "sleep", traceEvGoSleep, 2)
	} else {
		if *pt == t {
			*pt = nil
		}
		unlock(&timers.lock)
	}
	bAlert = *locked&0x100 != 0
	*locked = 0
	return
}


///////////////////////////////////////////////////////////////////////////////////////
//sleeper.go

package time

import "runtime"
import "sync/atomic"

type Sleeper struct {
	t    *runtimeTimer
	lock int32
}

func sleeptimer(pt **runtimeTimer, ns int64, lock *int32) (bAlert bool)

func (s *Sleeper) Reset() {
	s.lock &= ^0x100
}
func (s *Sleeper) stop() bool {
	t := s.t
	if t != nil {
		s.t = nil
		return stopTimer(t)
	}
	return false
}
func (s *Sleeper) Sleep(ns Duration) (bAlert bool) {
	a := s.lock &^ 0xFF
	atomic.CompareAndSwapInt32(&s.lock, a, a|0x201)
	bAlert = sleeptimer(&s.t, int64(ns), &s.lock)
	return
}
func (s *Sleeper) LockAlert() bool {
	s.Lock()
	t := s.t
	if t != nil {
		s.t = nil
		f := t.f
		arg := t.arg
		if stopTimer(t) {
			s.lock = 0x100
			f(arg, t.seq)
			return true
		}
	}
	s.lock = 0x100
	return false
}
func (s *Sleeper) Lock() {
	for {
		a := s.lock &^ 0xFF
		if atomic.CompareAndSwapInt32(&s.lock, a, a|0x201) {
			return
		}
		runtime.Gosched()
	}
}
func (s *Sleeper) Unlock() {
	s.lock = 0
}
func (s *Sleeper) IsAlert() bool {
	return s.t == nil
}
//////////////////////////////////////////////////////////////////////////////////
//test.go

func main() {
	var g time.Sleeper
	g.Reset()
	go func() {
		time.Sleep(time.Second * 5)
		fmt.Println(g.LockAlert())
	}()
	fmt.Println("WaitForSingle", g.Sleep(-1), g.IsAlert())

	g.Reset()
	go func() {
		time.Sleep(time.Second * 6)
		fmt.Println(g.LockAlert())
	}()
	fmt.Println("WaitForSingle", g.Sleep(time.Second*5), g.IsAlert())
}
