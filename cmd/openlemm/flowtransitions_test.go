package main

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/benemon/openlemm/internal/level"
	"github.com/benemon/openlemm/internal/screen"
)

var kindPhase = map[string]phase{
	"title": titleMenu, "code": codeEntry, "prelevel": preLevel, "results": results, "congrats": congrats,
}

// settle runs idle frames until no fade-out is running or level ending pending, then enough for a
// fade-in to finish.
func settle(t *testing.T, f *flow) {
	t.Helper()
	for n := 0; f.leaving || f.ph == playing && f.play.sim.TimeUp; n++ {
		if n > 1000 {
			t.Fatalf("still fading out after %d frames (phase %d)", n, f.ph)
		}
		run(t, f, input{}, 1)
	}
	run(t, f, input{}, 40)
}

// openAt opens the screen a fixture shows, with the session its state records.
func openAt(t *testing.T, f *flow, p *flowFixture) {
	st := &p.State
	f.sess.level, f.sess.effects, f.sess.cheat, f.sess.show = st.Level, st.MusicMode == "effects", st.Cheat, st.Show
	switch p.Kind {
	case "prelevel":
		l, err := level.LoadPlay(f.disc, st.Level)
		if err != nil {
			t.Fatal(err)
		}
		f.lvl = l
	case "results":
		r := st.Results
		f.result = screen.Results{TimeUp: r.Header == "time-up", Needed: r.Needed, Rescued: r.Rescued, Next: r.Played + 1}
	}
	f.enter(kindPhase[p.Kind])
	if p.Kind == "code" {
		copy(f.field[:], st.Code.Field)
		f.place = st.Code.Place
		if st.Code.Stage == "result" {
			f.match()
		}
	}
	run(t, f, input{}, 1)
}

// give applies one recorded input: a click is the button down
// for one frame, a key down for one frame; each is followed by idle frames until the screen
// has settled.
func give(t *testing.T, f *flow, in string) {
	var x, y int
	switch {
	case in == "START (mouse)" || in == "NEW LEVEL (mouse)":
		return // labels for the click before them
	case in == "left click":
		run(t, f, input{left: true}, 1)
	case in == "right click":
		run(t, f, input{right: true}, 1)
	case strings.HasPrefix(in, "mouse to"):
		if _, err := fmt.Sscanf(in, "mouse to (%d, %d), left click", &x, &y); err != nil {
			t.Fatalf("input %q: %v", in, err)
		}
		f.mx, f.my = float64(x), float64(y)
		f.clampPointer()
		run(t, f, input{left: true}, 1)
	case strings.HasPrefix(in, "key &"):
		var k int
		fmt.Sscanf(in, "key &%X", &k)
		var i input
		switch {
		case k >= 0x04 && k <= 0x1D:
			i.letters = 1 << (k - 0x04)
		case k == 0x28:
			i.ret = true
		case k == 0x29:
			i.escape = true
		case k == 0x2C:
			i.space = true
		default:
			t.Fatalf("input %q: key not handled", in)
		}
		run(t, f, i, 1)
	default:
		t.Fatalf("input %q not handled", in)
	}
	run(t, f, input{}, 1)
	settle(t, f)
}

// TestFlowTransitions replays every transition between two flow fixtures of the screens it can
// open at a recorded state (not the splashes, which TestBoot follows frame by frame): it opens the
// previous grab's screen with its state, gives the recorded inputs, and checks the flow reaches
// this grab's screen with the same state. The RAM writes are the sweep's forcing: the saved
// count is written into the level in play; the finished flag stands for the level finishing,
// so the level is left as at the end of its ending sequence; the level and cheat writes before a
// cheat skip set the level in play and cheat mode, which the sweep turned off again afterwards.
func TestFlowTransitions(t *testing.T) {
	a, err := loadAssets(testDisc(t))
	if err != nil {
		t.Fatal(err)
	}
	fx := loadFlowFixtures(t)
	screens := 0
	for n, f := range fx {
		if _, ok := kindPhase[f.Kind]; ok {
			screens++
		} else {
			delete(fx, n)
		}
	}
	if screens != 514 {
		t.Fatalf("%d flow fixtures of the screens a transition opens, want 514", screens)
	}
	var names []string
	for n, f := range fx {
		if fx[f.Transition.Previous] != nil {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	ok := map[string]int{}
	for _, n := range names {
		g, p := fx[n], fx[fx[n].Transition.Previous]
		f := newFlow(a, nil)
		openAt(t, f, p)
		writes := map[string]int{}
		for _, w := range g.Transition.Writes {
			writes[w.Addr] = w.Value // the last write to each address
		}
		skip := g.Transition.Via == "skip"
		finished := false
		forced := false
		force := func() {
			if f.ph != playing || forced {
				return
			}
			forced = true
			if v, ok := writes["&19C5C"]; ok {
				f.play.sim.Saved = v
			}
			_, finished = writes["&19D25"]
			if skip {
				f.play.n, f.sess.level, f.sess.cheat = writes["&19D44"], writes["&19D44"], true
			}
		}
		for _, in := range g.Transition.Inputs {
			force()
			give(t, f, in)
			if skip && f.ph != playing {
				f.sess.cheat = false
			}
		}
		force()
		if finished && f.ph == playing {
			f.levelOver()
			settle(t, f)
		}
		if msg := sameState(f, g); msg != "" {
			t.Errorf("%s from %s: %s", n, p.Grab, msg)
			continue
		}
		ok[g.Kind]++
	}
	t.Logf("%d transitions replayed: %v", len(names), ok)
}

// sameState compares the flow with a fixture's screen kind and state; "" when they agree.
func sameState(f *flow, g *flowFixture) string {
	st := &g.State
	if f.ph != kindPhase[g.Kind] {
		return fmt.Sprintf("at phase %d, want %s", f.ph, g.Kind)
	}
	if f.sess.cheat != st.Cheat || f.sess.show != st.Show || f.sess.effects != (st.MusicMode == "effects") {
		return fmt.Sprintf("cheat %v show %v effects %v, want %v %v %s", f.sess.cheat, f.sess.show, f.sess.effects, st.Cheat, st.Show, st.MusicMode)
	}
	switch g.Kind {
	case "results":
		r, res := st.Results, f.result
		code := 0
		if r.Code != nil {
			code = r.Code.Row
		}
		switch {
		case res.Next-1 != r.Played:
			return fmt.Sprintf("played %d, want %d", res.Next-1, r.Played)
		case res.TimeUp != (r.Header == "time-up"):
			return fmt.Sprintf("time-up %v, header %s", res.TimeUp, r.Header)
		case res.Rescued != r.Rescued || res.Needed != r.Needed:
			return fmt.Sprintf("rescued %d%% needed %d%%, want %d%% %d%%", res.Rescued, res.Needed, r.Rescued, r.Needed)
		case r.Verdict != nil && screen.Verdict(res.Rescued, res.Needed) != *r.Verdict:
			return fmt.Sprintf("verdict %d, want %d", screen.Verdict(res.Rescued, res.Needed), *r.Verdict)
		case res.Passed() && res.Next != 120 && res.Next != code:
			return fmt.Sprintf("code row %d, want %d", res.Next, code)
		case (r.Outcome == "fail") == res.Passed():
			return fmt.Sprintf("passed %v, outcome %s", res.Passed(), r.Outcome)
		}
	case "code":
		c := st.Code
		if string(f.field[:]) != c.Field || c.Stage == "entry" && (f.place != c.Place || f.res != screen.Typing) {
			return fmt.Sprintf("field %q at %d, want %q at %d", f.field, f.place, c.Field, c.Place)
		}
		if c.Stage == "result" && (f.res == screen.Typing || f.menu.Match(f.field) != max(c.Match, 0)) {
			return fmt.Sprintf("result %d, want match %d", f.res, c.Match)
		}
	case "title":
		if f.sess.level/30 != st.Title.RatingSign {
			return fmt.Sprintf("rating %d, want sign %d", f.sess.level/30, st.Title.RatingSign)
		}
		if red := f.border > 0; red != (g.Border == "000F") {
			return fmt.Sprintf("red border %v, fixture border %s", red, g.Border)
		}
	}
	if f.sess.level != st.Level {
		return fmt.Sprintf("level %d, want %d", f.sess.level, st.Level)
	}
	if g.Kind == "prelevel" && (f.sess.level%30+1 != st.Prelevel.Label || f.lvl.Title != st.Prelevel.Title) {
		return fmt.Sprintf("label %d title %q, want %d %q", f.sess.level%30+1, f.lvl.Title, st.Prelevel.Label, st.Prelevel.Title)
	}
	return ""
}
