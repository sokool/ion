package ion_test

import (
	"errors"
	"testing"

	"github.com/sokool/ion"
)

func TestErr(t *testing.T) {
	var (
		Err0 = ion.Errorf("")
		Err1 = Err0.New("err1")
		Err2 = ion.Errorf("%w:err2", Err1)
		Err3 = ion.Errorf("err3")
		Err4 = ion.Errorf("%w: oh no", Err2)
		Err5 = Err2.New("hi %w", Err3)
		Err6 = ion.Errorf("err6").New("err7").Wrap(Err2, Err3)
	)
	if s := Err0.New("a").Error(); s != "a" {
		t.Fatalf("expecting a, got %s", s)
	}
	if !Err0.In(Err1) {
		t.Fatal()
	}
	if !Err1.In(Err2) {
		t.Fatal()
	}
	if !Err1.In(Err4) {
		t.Fatal()
	}
	if !Err1.In(Err6) {
		t.Fatal()
	}
	if !Err2.In(Err4) {
		t.Fatal()
	}
	if !Err3.In(Err5) {
		t.Fatal()
	}
	if Err4.In(Err3) {
		t.Fatal()
	}
	if Err5.Error() != "err1:err2:hi err3" {
		t.Fatal()
	}
	if Err6.Error() != "err6:err7:err1:err2:err3" {
		t.Fatal()
	}
}

func TestErrJoin(t *testing.T) {
	var (
		ErrFoo      = ion.Errorf("foo")
		ErrStorage  = ion.Errorf("storage")
		ErrPostgres = ion.Errorf("postgres")
		Err         = errors.Join(ErrStorage, ErrPostgres)
	)
	if !ErrStorage.In(Err) {
		t.Fatalf("expecting storage, got %v", Err)
	}
	if !ErrPostgres.In(Err) {
		t.Fatalf("expecting postgres, got %v", Err)
	}
	if ErrFoo.In(Err) {
		t.Fatalf("not expecting foo")
	}
}
