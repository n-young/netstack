package tcp_test

import (
	"bytes"
	"math"
	"testing"

	tcp "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/tcp"
)

func TestCircBuffBasic(t *testing.T) {
	cb := tcp.NewCircBuff(10, 0)
	_, err := cb.PushData(0, []byte{1, 2, 3, 4, 5, 6})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 4 {
		t.Fatalf("should have had window size 4, had window size %d", cb.GetWindowSize(false))
	}
}

func TestCircBuffPushNone(t *testing.T) {
	cb := tcp.NewCircBuff(10, 0)
	_, err := cb.PushData(0, []byte{})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 10 {
		t.Fatalf("should have had window size 10, had window size %d", cb.GetWindowSize(false))
	}
}

func TestCircBuffPushFill(t *testing.T) {
	cb := tcp.NewCircBuff(10, 0)
	_, err := cb.PushData(0, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 0 {
		t.Fatalf("should have had window size 0, had window size %d", cb.GetWindowSize(false))
	}
}

func TestCircBuffBasicStartWack(t *testing.T) {
	cb := tcp.NewCircBuff(10, 128)
	_, err := cb.PushData(128, []byte{1, 2, 3, 4, 5, 6})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 4 {
		t.Fatalf("should have had window size 4, had window size %d", cb.GetWindowSize(false))
	}
}

func TestCircBuffPushNoneStartWack(t *testing.T) {
	cb := tcp.NewCircBuff(10, 128)
	_, err := cb.PushData(128, []byte{})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 10 {
		t.Fatalf("should have had window size 10, had window size %d", cb.GetWindowSize(false))
	}
}

func TestCircBuffPushFillStartWack(t *testing.T) {
	cb := tcp.NewCircBuff(10, 128)
	_, err := cb.PushData(128, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 0 {
		t.Fatalf("should have had window size 0, had window size %d", cb.GetWindowSize(false))
	}
}

func TestCircBuffBasicStartIntOverflow(t *testing.T) {
	cb := tcp.NewCircBuff(10, math.MaxUint32-4)
	_, err := cb.PushData(math.MaxUint32-4, []byte{1, 2, 3, 4, 5})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 5 {
		t.Fatalf("should have had window size 5, had window size %d", cb.GetWindowSize(false))
	}
}

func TestCircBuffPushNoneStartIntOverflow(t *testing.T) {
	cb := tcp.NewCircBuff(10, math.MaxUint32-4)
	_, err := cb.PushData(math.MaxUint32-4, []byte{})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 10 {
		t.Fatalf("should have had window size 10, had window size %d", cb.GetWindowSize(false))
	}
}

func TestCircBuffPushFillStartIntOverflow(t *testing.T) {
	cb := tcp.NewCircBuff(10, math.MaxUint32-4)
	_, err := cb.PushData(math.MaxUint32-4, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 0 {
		t.Fatalf("should have had window size 0, had window size %d", cb.GetWindowSize(false))
	}
}

func TestCircBuffPushFillStartIntOverflowTwice(t *testing.T) {
	cb := tcp.NewCircBuff(10, math.MaxUint32-3)
	_, err := cb.PushData(math.MaxUint32-3, []byte{0, 1, 2, 3, 4})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 5 {
		t.Fatalf("should have had window size 5, had window size %d", cb.GetWindowSize(false))
	}

	_, err = cb.PushData(1, []byte{5, 6, 7, 8, 9})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 0 {
		t.Fatalf("should have had window size 0, had window size %d", cb.GetWindowSize(false))
	}
}

func TestCircBuffManyPushes(t *testing.T) {
	cb := tcp.NewCircBuff(10, 128)
	_, err := cb.PushData(128, []byte{0, 1, 2})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 7 {
		t.Fatalf("should have had window size 7, had window size %d", cb.GetWindowSize(false))
	}

	_, err = cb.PushData(131, []byte{3, 4, 5})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 4 {
		t.Fatalf("should have had window size 4, had window size %d", cb.GetWindowSize(false))
	}
}

func TestCircBuffFragment(t *testing.T) {
	cb := tcp.NewCircBuff(10, 128)
	_, err := cb.PushData(128, []byte{0, 1, 2})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 7 {
		t.Fatalf("should have had window size 7, had window size %d", cb.GetWindowSize(false))
	}

	_, err = cb.PushData(132, []byte{4, 5, 6})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 7 {
		t.Fatalf("should have had window size 7, had window size %d", cb.GetWindowSize(false))
	}

	_, err = cb.PushData(131, []byte{3})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 3 {
		t.Fatalf("should have had window size 3, had window size %d", cb.GetWindowSize(false))
	}
}

func TestCircBuffFragments(t *testing.T) {
	cb := tcp.NewCircBuff(10, 128)
	_, err := cb.PushData(128, []byte{0, 1, 2})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 7 {
		t.Fatalf("should have had window size 7, had window size %d", cb.GetWindowSize(false))
	}

	_, err = cb.PushData(132, []byte{4})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 7 {
		t.Fatalf("should have had window size 7, had window size %d", cb.GetWindowSize(false))
	}

	_, err = cb.PushData(133, []byte{5})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 7 {
		t.Fatalf("should have had window size 7, had window size %d", cb.GetWindowSize(false))
	}

	_, err = cb.PushData(134, []byte{6})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 7 {
		t.Fatalf("should have had window size 7, had window size %d", cb.GetWindowSize(false))
	}

	_, err = cb.PushData(131, []byte{3})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 3 {
		t.Fatalf("should have had window size 3, had window size %d", cb.GetWindowSize(false))
	}
}

func TestCircBuffFragmentsLeftOver(t *testing.T) {
	cb := tcp.NewCircBuff(10, 128)
	_, err := cb.PushData(128, []byte{0, 1, 2})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 7 {
		t.Fatalf("should have had window size 7, had window size %d", cb.GetWindowSize(false))
	}

	_, err = cb.PushData(132, []byte{4})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 7 {
		t.Fatalf("should have had window size 7, had window size %d", cb.GetWindowSize(false))
	}

	_, err = cb.PushData(134, []byte{6})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 7 {
		t.Fatalf("should have had window size 7, had window size %d", cb.GetWindowSize(false))
	}

	_, err = cb.PushData(131, []byte{3})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 5 {
		t.Fatalf("should have had window size 5, had window size %d", cb.GetWindowSize(false))
	}
}

func TestCircBuffPullTooMuch(t *testing.T) {
	cb := tcp.NewCircBuff(10, 0)
	_, err := cb.PushData(0, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 0 {
		t.Fatalf("should have had window size 0, had window size %d", cb.GetWindowSize(false))
	}

	_, err = cb.PullData(11)
	if err == nil {
		t.Fatal("should have not pulled data")
	}
}

func TestCircBuffPullTooMuchOverflow(t *testing.T) {
	cb := tcp.NewCircBuff(10, math.MaxUint32-4)
	_, err := cb.PushData(math.MaxUint32-4, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 0 {
		t.Fatalf("should have had window size 0, had window size %d", cb.GetWindowSize(false))
	}

	_, err = cb.PullData(11)
	if err == nil {
		t.Fatal("should have not pulled data")
	}
}

func TestCircBuffPullOverflow(t *testing.T) {
	cb := tcp.NewCircBuff(10, math.MaxUint32-4)
	_, err := cb.PushData(math.MaxUint32-4, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 0 {
		t.Fatalf("should have had window size 0, had window size %d", cb.GetWindowSize(false))
	}

	data, err := cb.PullData(3)
	t.Log(cb.PrintState())
	if !bytes.Equal(data, []byte{0, 1, 2}) {
		t.Log(data)
		t.Fatal("pulled wrong data")
	}

	data, err = cb.PullData(5)
	t.Log(cb.PrintState())
	if !bytes.Equal(data, []byte{3, 4, 5, 6, 7}) {
		t.Log(data)
		t.Fatal("pulled wrong data")
	}
}

func TestCircBuffPullOverflow2(t *testing.T) {
	cb := tcp.NewCircBuff(16, math.MaxUint32-4)
	_, err := cb.PushData(math.MaxUint32-4, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	t.Log(cb.PrintState())
	if err != nil {
		t.Fatal(err)
	}
	if cb.GetWindowSize(false) != 6 {
		t.Fatalf("should have had window size 6, had window size %d", cb.GetWindowSize(false))
	}

	data, err := cb.PullData(5)
	t.Log(cb.PrintState())
	if !bytes.Equal(data, []byte{0, 1, 2, 3, 4}) {
		t.Log(data)
		t.Fatal("pulled wrong data")
	}

	data, err = cb.PullData(5)
	t.Log(cb.PrintState())
	if !bytes.Equal(data, []byte{5, 6, 7, 8, 9}) {
		t.Log(data)
		t.Fatal("pulled wrong data")
	}
}
