package ast

import (
	"fmt"
	"github.com/NicoNex/tau/obj"
)

type Times struct {
	l Node
	r Node
}

func NewTimes(l, r Node) Node {
	return Times{l, r}
}

func (t Times) Eval(env *obj.Env) obj.Object {
	var left = t.l.Eval(env)
	var right = t.r.Eval(env)

	if isError(left) {
		return left
	}
	if isError(right) {
		return right
	}

	if left.Type() != right.Type() {
		return obj.NewError(
			"invalid operation %v * %v (mismatched types %v and %v)",
			left, right, left.Type(), right.Type(),
		)
	}

	switch left.Type() {
	case obj.INT:
		l := left.(*obj.Integer)
		r := right.(*obj.Integer)
		return obj.NewInteger(l.Val() * r.Val())

	case obj.FLOAT:
		l := left.(*obj.Float)
		r := right.(*obj.Float)
		return obj.NewFloat(l.Val() * r.Val())

	default:
		return obj.NewError("unsupported operator '*' for type %v", left.Type())
	}
}

func (t Times) String() string {
	return fmt.Sprintf("(%v * %v)", t.l, t.r)
}
