package ctxkey_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/ctxkey"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

// userID is how a consumer declares a key: one package-level variable.
var userID = ctxkey.New[int]("user-id")

func ExampleNew() {
	ctx := userID.With(context.Background(), 42)
	id, ok := userID.Value(ctx)
	fmt.Println(id, ok)
	fmt.Println(ctx)
	// Output:
	// 42 true
	// context.Background.WithValue(user-id, int)
}

func ExampleKey_Value() {
	id, ok := userID.Value(context.Background())
	fmt.Println(id, ok)
	// Output:
	// 0 false
}

func ExampleKey_With() {
	parent := userID.With(context.Background(), 1)
	child := userID.With(parent, 2)
	p, _ := userID.Value(parent)
	c, _ := userID.Value(child)
	fmt.Println(p, c)
	// Output:
	// 1 2
}

type point struct{ X, Y int }

func TestKey_RoundTrip(t *testing.T) {
	ctx := context.Background()

	ki := ctxkey.New[int]("int")
	if v, ok := ki.Value(ki.With(ctx, 7)); !ok || v != 7 {
		t.Errorf("int: got (%v, %v), want (7, true)", v, ok)
	}
	ks := ctxkey.New[string]("string")
	if v, ok := ks.Value(ks.With(ctx, "")); !ok || v != "" {
		t.Errorf("empty string: got (%q, %v), want (\"\", true)", v, ok)
	}
	kp := ctxkey.New[point]("point")
	if v, ok := kp.Value(kp.With(ctx, point{1, 2})); !ok || v != (point{1, 2}) {
		t.Errorf("struct: got (%v, %v), want ({1 2}, true)", v, ok)
	}
	kptr := ctxkey.New[*point]("ptr")
	want := &point{3, 4}
	if v, ok := kptr.Value(kptr.With(ctx, want)); !ok || v != want {
		t.Errorf("pointer: got (%p, %v), want (%p, true)", v, ok, want)
	}
	if v, ok := kptr.Value(kptr.With(ctx, nil)); !ok || v != nil {
		t.Errorf("nil pointer: got (%v, %v), want (nil, true)", v, ok)
	}
	ke := ctxkey.New[error]("err")
	if v, ok := ke.Value(ke.With(ctx, nil)); !ok || v != nil {
		t.Errorf("nil interface: got (%v, %v), want (nil, true): a stored nil is not a missing value", v, ok)
	}
	sentinel := errors.New("boom")
	if v, ok := ke.Value(ke.With(ctx, sentinel)); !ok || !errors.Is(v, sentinel) {
		t.Errorf("interface: got (%v, %v), want (boom, true)", v, ok)
	}
}

func TestKey_DistinctSameName(t *testing.T) {
	a := ctxkey.New[int]("same")
	b := ctxkey.New[int]("same")
	ctx := a.With(context.Background(), 1)
	if v, ok := b.Value(ctx); ok {
		t.Errorf("b read a's value: got (%v, true), want (0, false)", v)
	}
	ctx = b.With(ctx, 2)
	if v, _ := a.Value(ctx); v != 1 {
		t.Errorf("a after b.With: got %v, want 1", v)
	}
	if v, _ := b.Value(ctx); v != 2 {
		t.Errorf("b: got %v, want 2", v)
	}

	e1, e2 := ctxkey.New[struct{}](""), ctxkey.New[struct{}]("")
	if e1 == e2 {
		t.Fatal("two keys with the empty name and a zero-size T are the same pointer")
	}
	if _, ok := e2.Value(e1.With(context.Background(), struct{}{})); ok {
		t.Error("empty-name keys collide")
	}

	// Same name, different T: never the same key.
	s := ctxkey.New[string]("same")
	if v, ok := s.Value(ctx); ok {
		t.Errorf("Key[string] read a Key[int] value: got (%q, true)", v)
	}
}

func TestKey_Shadowing(t *testing.T) {
	k := ctxkey.New[string]("role")
	parent := k.With(context.Background(), "viewer")
	child := k.With(parent, "owner")
	other := ctxkey.New[int]("other")
	grandchild := other.With(child, 9) // an unrelated key between does not hide k

	if v, _ := k.Value(parent); v != "viewer" {
		t.Errorf("parent: got %q, want viewer (a child's With must not change its parent)", v)
	}
	if v, _ := k.Value(child); v != "owner" {
		t.Errorf("child: got %q, want owner", v)
	}
	if v, _ := k.Value(grandchild); v != "owner" {
		t.Errorf("grandchild: got %q, want owner", v)
	}
}

func TestKey_Missing(t *testing.T) {
	k := ctxkey.New[point]("missing")
	ctx := ctxkey.New[point]("present").With(context.Background(), point{1, 1})
	if v, ok := k.Value(ctx); ok || v != (point{}) {
		t.Errorf("got (%v, %v), want ({0 0}, false)", v, ok)
	}
	if v, ok := k.Value(context.TODO()); ok || v != (point{}) {
		t.Errorf("TODO: got (%v, %v), want ({0 0}, false)", v, ok)
	}
}

func TestKey_NilContext(t *testing.T) {
	k := ctxkey.New[int]("n")
	if v, ok := k.Value(nil); ok || v != 0 { //nolint:staticcheck // nil ctx is the input under test
		t.Errorf("Value(nil): got (%v, %v), want (0, false)", v, ok)
	}
	ctx := k.With(nil, 5) //nolint:staticcheck // nil ctx is the input under test
	if ctx == nil {
		t.Fatal("With(nil, v) returned a nil context")
	}
	if v, ok := k.Value(ctx); !ok || v != 5 {
		t.Errorf("With(nil, 5): got (%v, %v), want (5, true)", v, ok)
	}
	if ctx.Done() != nil {
		t.Error("With(nil, v) should derive from context.Background, which is never cancelled")
	}
}

func TestKey_NilKey(t *testing.T) {
	var k *ctxkey.Key[int]
	base := context.Background()
	if got := k.With(base, 1); got != base {
		t.Errorf("nil key With: got %v, want ctx unchanged", got)
	}
	if v, ok := k.Value(base); ok || v != 0 {
		t.Errorf("nil key Value: got (%v, %v), want (0, false)", v, ok)
	}
	if got := k.String(); got != "<nil>" {
		t.Errorf("nil key String: got %q, want <nil>", got)
	}
}

// TestKey_StringKeyCollision is the go_51 defect: a value stored under the plain string key "userID" is read
// (and overwritten) by every other package using the same string. A ctxkey.Key named "userID" neither sees nor
// is seen by that string key, nor by another package's ctxkey.Key of the same name.
func TestKey_StringKeyCollision(t *testing.T) {
	// Two packages using a string key, as go_51 does: the second silently overwrites the first.
	ctx := context.WithValue(context.Background(), "userID", 42) //nolint:staticcheck // the defect under test
	ctx = context.WithValue(ctx, "userID", "alice@example.com")  //nolint:staticcheck // the defect under test
	if _, ok := ctx.Value("userID").(int); ok {                  //nolint:staticcheck // the defect under test
		t.Fatal("expected the string key to have been overwritten (the go_51 defect)")
	}

	pkgA := ctxkey.New[int]("userID")
	pkgB := ctxkey.New[string]("userID")
	ctx = pkgA.With(ctx, 42)
	ctx = pkgB.With(ctx, "alice@example.com")
	ctx = context.WithValue(ctx, "userID", "mallory") //nolint:staticcheck // the defect under test

	if v, ok := pkgA.Value(ctx); !ok || v != 42 {
		t.Errorf("pkgA: got (%v, %v), want (42, true)", v, ok)
	}
	if v, ok := pkgB.Value(ctx); !ok || v != "alice@example.com" {
		t.Errorf("pkgB: got (%q, %v), want (alice@example.com, true)", v, ok)
	}
	if v := ctx.Value("userID"); v != "mallory" { //nolint:staticcheck // the defect under test
		t.Errorf("string key: got %v, want mallory (ctxkey keys must not answer a string lookup)", v)
	}
}

type stringerVal struct{}

func (stringerVal) String() string { return "S" }

func TestKey_String(t *testing.T) {
	k := ctxkey.New[int]("auth.user-id")
	if got := k.String(); got != "auth.user-id" {
		t.Errorf("String: got %q", got)
	}
	cases := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"int value prints its type only", k.With(context.Background(), 42),
			"context.Background.WithValue(auth.user-id, int)"},
		{"string value prints itself", ctxkey.New[string]("s").With(context.Background(), "v"),
			"context.Background.WithValue(s, v)"},
		{"stringer value prints String", ctxkey.New[stringerVal]("st").With(context.Background(), stringerVal{}),
			"context.Background.WithValue(st, S)"},
		{"nil interface prints <nil>", ctxkey.New[error]("e").With(context.Background(), nil),
			"context.Background.WithValue(e, <nil>)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := fmt.Sprint(c.ctx); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestKey_Concurrent(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	k := ctxkey.New[int]("c")
	ctx = k.With(ctx, 3)

	var wg sync.WaitGroup
	errs := make(chan string, 2*64) // each goroutine sends at most two
	for i := range 64 {
		wg.Go(func() {
			child := k.With(ctx, i)
			if v, ok := k.Value(child); !ok || v != i {
				errs <- fmt.Sprintf("child %d: got (%v, %v)", i, v, ok)
			}
			if v, ok := k.Value(ctx); !ok || v != 3 {
				errs <- fmt.Sprintf("parent seen from %d: got (%v, %v)", i, v, ok)
			}
		})
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

func BenchmarkKey_Value(b *testing.B) {
	k := ctxkey.New[int]("bench")
	ctx := k.With(context.Background(), 1)
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := k.Value(ctx); !ok {
			b.Fatal("missing")
		}
	}
}
