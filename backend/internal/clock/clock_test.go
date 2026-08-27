package clock

import (
	"testing"
	"time"
)

// TestOffsetIsZeroByDefault: the default is the system clock. If this ever
// fails, production is running on a travelled clock.
func TestOffsetIsZeroByDefault(t *testing.T) {
	if Offset() != 0 || Travelling() {
		t.Fatalf("offset %v, travelling %v — expected the system clock", Offset(), Travelling())
	}
	if d := time.Since(Now()); d > time.Second || d < -time.Second {
		t.Errorf("Now() is %v away from time.Now()", d)
	}
}

func TestSetAdvanceReset(t *testing.T) {
	t.Cleanup(Reset)

	ziel := time.Date(2031, time.July, 4, 12, 0, 0, 0, time.UTC)
	Set(ziel)
	if d := Now().Sub(ziel); d > time.Second || d < 0 {
		t.Errorf("after Set the clock is %v off", d)
	}
	if !Travelling() {
		t.Error("Travelling() is false although the clock was moved")
	}

	Advance(48 * time.Hour)
	if d := Now().Sub(ziel.Add(48 * time.Hour)); d > time.Second || d < 0 {
		t.Errorf("after Advance the clock is %v off", d)
	}

	// Time keeps flowing while travelled — nothing is frozen.
	erst := Now()
	time.Sleep(2 * time.Millisecond)
	if !Now().After(erst) {
		t.Error("the clock stands still while travelled")
	}

	Reset()
	if Offset() != 0 || Travelling() {
		t.Errorf("Reset left an offset of %v", Offset())
	}
}

// TestAdvanceBackwards: travelling back is allowed too — a test may need a
// completion that lies in the past without backdating it.
func TestAdvanceBackwards(t *testing.T) {
	t.Cleanup(Reset)
	Advance(-72 * time.Hour)
	if d := time.Since(Now()); d < 71*time.Hour {
		t.Errorf("clock only went back %v", d)
	}
}
