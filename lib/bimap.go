package lib

type BiMap[A comparable, B comparable] struct {
	a map[A]B
	b map[B]A
}

func (bm *BiMap[A, B]) GetB(a A) (B, bool) {
	val, ok := bm.a[a]
	return val, ok
}

func (bm *BiMap[A, B]) GetA(b B) (A, bool) {
	val, ok := bm.b[b]
	return val, ok
}

func (bm *BiMap[A, B]) Set(a A, b B) {
	if bm.a == nil {
		bm.a = make(map[A]B)
		bm.b = make(map[B]A)
	}
	bm.a[a] = b
	bm.b[b] = a
}

func (bm *BiMap[A, B]) DeleteA(a A) {
	if bm.a == nil {
		return
	}
	b := bm.a[a]
	delete(bm.a, a)
	delete(bm.b, b)
}

func (bm *BiMap[A, B]) DeleteB(b B) {
	if bm.a == nil {
		return
	}
	a := bm.b[b]
	delete(bm.a, a)
	delete(bm.b, b)
}

func (bm *BiMap[A, B]) ListA() []A {
	var l []A
	for a := range bm.a {
		l = append(l, a)
	}
	return l
}

func (bm *BiMap[A, B]) ListB() []B {
	var l []B
	for b := range bm.b {
		l = append(l, b)
	}
	return l
}
